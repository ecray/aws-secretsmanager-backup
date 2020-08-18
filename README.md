# AWS Secrets Manager Backup

This tool backs up the AWSCURRENT label in SecretsManager to an s3 bucket to allow for the 
possibility of restoring previous versions beyond the AWSPREVIOUS label (current + 1).

## Usage
```
Usage:
  -bucket string
        AWS S3 Bucket
  -region string
        AWS region
```
### You can also export environment variables
```
> export AWS_S3_BUCKET=secondary-backup-aebef AWS_REGION=eu-west-1
> ./run -h
Usage:
  -bucket string
        AWS S3 Bucket (default "secondary-backup-aebef")
  -region string
        AWS region (default "eu-west-1")
```

## Create secret and backup
```
> aws secretsmanager put-secret-value --secret-id my-test-creds --secret-string '{"hunger":true}'
{
    "ARN": "arn:aws:secretsmanager:eu-west-1:852706359288:secret:my-test-creds-MgUjYf",
    "Name": "my-test-creds",
    "VersionId": "d57bd1f1-7a49-46f7-928e-e8a7c39902d4",
    "VersionStages": [
        "AWSCURRENT"
    ]
}

> ./run
2020/08/16 10:20:41 Creating key: my-test-creds/d57bd1f1-7a49-46f7-928e-e8a7c39902d4
```

## Update secret and backup
```
> aws secretsmanager put-secret-value --secret-id my-test-creds --secret-string '{"hunger":true, "pants":false}'
{
    "ARN": "arn:aws:secretsmanager:eu-west-1:852706359288:secret:my-test-creds-MgUjYf",
    "Name": "my-test-creds",
    "VersionId": "19fe7368-c6b3-45c8-873b-d358de56453a",
    "VersionStages": [
        "AWSCURRENT"
    ]
}

> ./run
2020/08/16 10:20:55 Creating key: my-test-creds/19fe7368-c6b3-45c8-873b-d358de56453a

> ./run
2020/08/16 10:20:59 Backup is current for  my-test-creds
```

## Check backup contents
```
> aws s3 ls secondary-backup-aebef/my-test-creds/
2020-08-16 10:20:56         30 19fe7368-c6b3-45c8-873b-d358de56453a
2020-08-16 10:20:42         30 d57bd1f1-7a49-46f7-928e-e8a7c39902d4

> aws s3 cp s3://secondary-backup-aebef/my-test-creds/19fe7368-c6b3-45c8-873b-d358de56453a 19fe7368-c6b3-45c8-873b-d358de56453a
download: s3://secondary-backup-aebef/my-test-creds/19fe7368-c6b3-45c8-873b-d358de56453a to ./19fe7368-c6b3-45c8-873b-d358de56453a

> cat 19fe7368-c6b3-45c8-873b-d358de56453a
{"hunger":true, "pants":false}
```
