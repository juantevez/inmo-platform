package ports

import "context"

// MediaStorageProvider define el contrato para generar URLs prefirmadas de carga.
// El adaptador concreto (S3, GCS, MinIO) implementa esta interfaz.
type MediaStorageProvider interface {
	GeneratePresignedURL(ctx context.Context, propertyID, filename, contentType string) (presignedURL string, finalURL string, err error)
}
