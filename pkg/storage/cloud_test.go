package storage_test

import (
	"testing"

	"github.com/cuiwenguang/go-saas-lite/pkg/storage"
)

func TestCloudDriversValidateOptions(t *testing.T) {
	if _, err := storage.NewCOS(storage.COSOptions{}); err == nil {
		t.Fatal("cos should reject empty options")
	}
	if _, err := storage.NewCOS(storage.COSOptions{Bucket: "b", SecretID: "id", SecretKey: "key"}); err == nil {
		t.Fatal("cos should require region when endpoint is empty")
	}
	if _, err := storage.NewOSS(storage.OSSOptions{}); err == nil {
		t.Fatal("oss should reject empty options")
	}
	if _, err := storage.NewOSS(storage.OSSOptions{Bucket: "b", Endpoint: "https://oss-cn-hangzhou.aliyuncs.com"}); err == nil {
		t.Fatal("oss should require access key")
	}
}

func TestCloudDriversConstruct(t *testing.T) {
	cos, err := storage.NewCOS(storage.COSOptions{
		Bucket:    "demo-1250000000",
		Region:    "ap-guangzhou",
		SecretID:  "id",
		SecretKey: "key",
	})
	if err != nil || cos == nil {
		t.Fatalf("new cos: %v", err)
	}

	oss, err := storage.NewOSS(storage.OSSOptions{
		Bucket:          "demo",
		Endpoint:        "https://oss-cn-hangzhou.aliyuncs.com",
		AccessKeyID:     "id",
		AccessKeySecret: "secret",
	})
	if err != nil || oss == nil {
		t.Fatalf("new oss: %v", err)
	}
}
