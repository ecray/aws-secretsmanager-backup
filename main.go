package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"io"
	"log"
	"os"
	"strings"
)

type AwsClient struct {
	Acct AwsAccount
	SecretsManager  *secretsmanager.SecretsManager
	s3Client *s3.S3
	s3Uploader *s3manager.Uploader
	Bucket string
}

type AwsAccount struct {
	Region          string
	Session         *session.Session
}

type Secret struct {
	Key string
	Content *secretsmanager.GetSecretValueOutput
}

var (
	client = AwsClient{}
)

func init() {
	flagset := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	flagset.StringVar(&client.Acct.Region, "region", os.Getenv("AWS_REGION"), "AWS region")
	flagset.StringVar(&client.Bucket, "bucket", os.Getenv("AWS_S3_BUCKET"), "AWS S3 Bucket")

	flagset.Parse(os.Args[1:])
}


func main() {
	os.Exit(Main())
}

func Main() int {

	// Create session
	client.awsConfig()

	client.SecretsManager = secretsmanager.New(client.Acct.Session)

	// Fetch list of secrets
	smList, err := client.getSecretsList()
	if err != nil {
		log.Fatalln(err)
	}

	client.s3Client = s3.New(client.Acct.Session)

	for _, s := range smList {
		var secret Secret

		input := &secretsmanager.GetSecretValueInput{
			SecretId:     aws.String(s),
			VersionStage: aws.String("AWSCURRENT"),
		}

		// Get SecretString for each value in list
		secret.Content, err = client.SecretsManager.GetSecretValue(input)
		// Log error, and continue to next secret
		// This should handle secrets that have no value
		// and other failures that are not fatal
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				case secretsmanager.ErrCodeResourceNotFoundException:
					log.Println("Error fetching", s, aerr.Error())
				default:
					log.Println(aerr.Error())
				}
			}
			continue
		}

		// Build key string ie my-secret/aaa-bbb-ccc-ddd
		secret.Key = strings.Join([]string{*secret.Content.Name, *secret.Content.VersionId}, "/")

		// Check s3 bucket and prefix exist
		objList, err := client.getObjectsList(s)

		if err != nil {
			log.Fatalln(err)
		}

		// Check if version-id object exists in bucket
		found := false
		for _, o := range objList {
			if o == secret.Key {
				log.Println("Backup is current for ", s)
				found = true
				break
			}
		}

		if ! found {
			log.Println("Creating key: ", secret.Key)
			if err = client.uploadSecret(secret); err != nil {
				log.Fatalln(err)
			}
		}
	}

	log.Println("Backup job completed.")

	return 0
}

func (c *AwsClient) awsConfig() {
	// Create valid AWS session
	c.Acct.Session = session.Must(session.NewSessionWithOptions(session.Options{
		// Without this thing, OIDC roles don't work
		SharedConfigState: session.SharedConfigEnable,
		Config: aws.Config{
			S3ForcePathStyle: aws.Bool(true),
		},
	}))
}

func (c *AwsClient) getSecretsList() ([]string, error) {
	var smList []string

	lsi := &secretsmanager.ListSecretsInput{}
	err := client.SecretsManager.ListSecretsPages(lsi,
		func(page *secretsmanager.ListSecretsOutput, lastPage bool) bool {
			var nextPage bool = false
			for _, p := range page.SecretList {
				smList = append(smList, *p.Name)
			}

			if page.NextToken != nil {
				lsi.SetNextToken(*page.NextToken)
				nextPage = true
			}
			return nextPage
		})

	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			// Get error details
			return smList, fmt.Errorf("Error Code: %v Message: %v\n", awsErr.Code(), awsErr.Message())
		}
	}

	if len(smList) == 0 {
		return smList, fmt.Errorf(("No secrets found. Verify account and permissions."))
	}

	return smList, nil
}

func (c *AwsClient) getObjectsList(prefix string) ([]string, error) {
	var files []string

	req := &s3.ListObjectsV2Input{Bucket: aws.String(c.Bucket), Prefix: aws.String(prefix)}
	err := c.s3Client.ListObjectsV2Pages(req, func(resp *s3.ListObjectsV2Output, lastPage bool) bool {
		for _, content := range resp.Contents {
			files = append(files, *content.Key)
		}
		return true
	})
	if err != nil {
		return files, fmt.Errorf("failed to list objects for bucket %v: %v", c.Bucket, err)
	}
	return files, nil
}

func (c *AwsClient) uploadSecret(s Secret) error {
	// Chunk uploads
	c.s3Uploader = s3manager.NewUploader(c.Acct.Session, func(u *s3manager.Uploader) {
		u.PartSize = 64 * 1024 * 1024 // 64MB per part
		})

	// Convert string using bytes Reader to use with io.ReadSeeker Reader interface
	f := bytes.NewReader([]byte(*s.Content.SecretString))
	_, err := c.s3Uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(c.Bucket),
		Key: aws.String(s.Key),
		Body: io.ReadSeeker(f),
	})
	if err != nil {
		return fmt.Errorf("unable to upload %q to %q, %v", s.Key, c.Bucket, err)
	}

	return nil
}