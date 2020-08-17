package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"io"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
)


/*
AWS permissions needed
secretsmanager:ListSecrets
secretsmanager:ListSecretVersionIds
secretsmanager:GetSecretValue
kms:Decrypt
s3:Write
*/

type AwsClient struct {
	Acct AwsAccount
	SecretsManager  *secretsmanager.SecretsManager
	s3Client *s3.S3
	Bucket string
}

type AwsAccount struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
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
	client.initSmClient()

	// Fetch list of secrets
	sm, err := client.SecretsManager.ListSecrets(&secretsmanager.ListSecretsInput{})
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			// Get error details
			log.Fatalln("Error:", awsErr.Code(), awsErr.Message())
		}
	}

	if sm.SecretList == nil {
		log.Fatalln("No secrets found. Verify account and permissions.")
	}

	client.initS3Client()
	for _, s := range sm.SecretList {
		var secret Secret

		input := &secretsmanager.GetSecretValueInput{
			SecretId:     aws.String(*s.Name),
			VersionStage: aws.String("AWSCURRENT"),
		}

		// Get SecretString for each value in list
		secret.Content , err = client.SecretsManager.GetSecretValue(input)
		if err != nil {
			log.Fatalf("Failed to fetch secret %v\n", *s.Name)
		}

		// Build key string ie my-secret/aaa-bbb-ccc-ddd
		secret.Key = strings.Join([]string{*secret.Content.Name, *secret.Content.VersionId}, "/")

		// Check s3 bucket and prefix exist
		objList, err := client.getObjectsList(*s.Name)

		if err != nil {
			log.Fatalln(err)
		}

		// Check if version-id object exists in bucket
		found := false
		for _, o := range objList {
			if o == secret.Key {
				log.Println("Backup is current for ", *s.Name)
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

	return 0
}

func (c *AwsClient) initSmClient() {
	// session.Must handles setup for creating a valid session
	sess := session.Must(session.NewSession(&aws.Config{
		Credentials:      credentials.NewEnvCredentials(),
	}))

	c.SecretsManager = secretsmanager.New(sess)
}

func (c *AwsClient) initS3Client() {
	// session.Must handles setup for creating a valid session
	sess := session.Must(session.NewSession(&aws.Config{
		Credentials:      credentials.NewEnvCredentials(),
		S3ForcePathStyle: aws.Bool(true),
	}))

	c.s3Client = s3.New(sess)
}

func (c *AwsClient) getObjectsList(prefix string) ([]string, error) {
	//StartAfter might be helpful for time-based retrieval
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
	// Setup the S3 Upload Manager. Also see the SDK doc for the Upload Manager
	// for more information on configuring part size, and concurrency.
	// http://docs.aws.amazon.com/sdk-for-go/api/service/s3/s3manager/#NewUploader

	sess := session.Must(session.NewSession(&aws.Config{
		Credentials:      credentials.NewEnvCredentials(),
		S3ForcePathStyle: aws.Bool(true),
	}))

	uploader := s3manager.NewUploader(sess, func(u *s3manager.Uploader) {
		u.PartSize = 64 * 1024 * 1024 // 64MB per part
		})

	// Convert string using bytes Reader to use with io.ReadSeeker Reader interface
	f := bytes.NewReader([]byte(*s.Content.SecretString))
	_, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(c.Bucket),
		Key: aws.String(s.Key),
		Body: io.ReadSeeker(f),
	})
	if err != nil {
		return fmt.Errorf("unable to upload %q to %q, %v", s.Key, c.Bucket, err)
	}

	return nil
}