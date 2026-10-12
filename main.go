package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func main() {
	endpoint := envOr("MINIO_ENDPOINT", "minio:9000")
	accessKey := envOr("MINIO_ACCESS_KEY", "minioadmin")
	secretKey := envOr("MINIO_SECRET_KEY", "minioadmin")
	bucket := envOr("MINIO_BUCKET", "storage")

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: false,
	})
	if err != nil {
		log.Fatalf("minio client: %v", err)
	}

	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		log.Fatalf("bucket check: %v", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			log.Fatalf("make bucket: %v", err)
		}
	}

	srv := &server{client: client, bucket: bucket}
	m := newMetrics()

	http.HandleFunc("/", srv.index)
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	http.HandleFunc("/metrics", m.handler)
	http.HandleFunc("/objects", m.instrument("list", srv.list))
	http.HandleFunc("/objects/", m.instrument("object", srv.object))

	addr := ":" + envOr("PORT", "8080")
	log.Printf("listening on %s (bucket=%s, endpoint=%s)", addr, bucket, endpoint)
	log.Fatal(http.ListenAndServe(addr, nil))
}

type server struct {
	client *minio.Client
	bucket string
}

func (s *server) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Recursive: true}) {
		if obj.Err != nil {
			http.Error(w, obj.Err.Error(), http.StatusInternalServerError)
			return
		}
		io.WriteString(w, obj.Key+"\n")
	}
}

func (s *server) object(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/objects/")
	if key == "" {
		http.Error(w, "missing object key", http.StatusBadRequest)
		return
	}
	ctx := r.Context()

	switch r.Method {
	case http.MethodPut, http.MethodPost:
		_, err := s.client.PutObject(ctx, s.bucket, key, r.Body, r.ContentLength,
			minio.PutObjectOptions{ContentType: r.Header.Get("Content-Type")})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	case http.MethodGet:
		obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer obj.Close()
		if _, err := io.Copy(w, obj); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case http.MethodDelete:
		if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
