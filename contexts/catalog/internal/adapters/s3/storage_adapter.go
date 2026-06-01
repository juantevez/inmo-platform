package s3adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"inmo.platform/shared/pkg/apperr"
)

type StorageAdapter struct {
	presignClient *s3.PresignClient
	bucket        string
	region        string
}

// NewStorageAdapter crea el adaptador de S3 leyendo AWS_REGION y AWS_BUCKET_NAME del entorno.
// Las credenciales se resuelven via la cadena estándar de AWS (env vars, ~/.aws/credentials, IAM role).
func NewStorageAdapter(ctx context.Context, bucket, region string) (*StorageAdapter, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, apperr.NewInternal("no se pudo cargar la configuración de AWS", err)
	}

	s3Client := s3.NewFromConfig(cfg)
	presignClient := s3.NewPresignClient(s3Client)

	return &StorageAdapter{
		presignClient: presignClient,
		bucket:        bucket,
		region:        region,
	}, nil
}

// GeneratePresignedURL genera una URL prefirmada para un PUT directo a S3 (válida 5 minutos).
// Devuelve también la URL pública final que tendrá el archivo una vez subido.
func (a *StorageAdapter) GeneratePresignedURL(ctx context.Context, propertyID, filename, contentType string) (presignedURL string, finalURL string, err error) {
	key := fmt.Sprintf("properties/%s/%s", propertyID, filename)

	req, err := a.presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(a.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, s3.WithPresignExpires(5*time.Minute))
	if err != nil {
		return "", "", apperr.NewInternal("error al generar la URL prefirmada de S3", err)
	}

	final := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", a.bucket, a.region, key)
	return req.URL, final, nil
}
