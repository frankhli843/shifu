package minio

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/edgenesis/shifu/pkg/deviceshifu/deviceshifubase"
	"github.com/edgenesis/shifu/pkg/k8s/api/v1alpha1"
	"github.com/edgenesis/shifu/pkg/logger"
	"github.com/edgenesis/shifu/pkg/telemetryservice/utils"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"io"
	"net/http"
	"os"
)

func BindMinIOServiceHandler(w http.ResponseWriter, r *http.Request) {
	// Parse request content
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Errorf("Error when Read Data From Body, error: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	logger.Infof("requestBody: %s", string(body))
	request := v1alpha1.TelemetryRequest{}

	err = json.Unmarshal(body, &request)
	if err != nil {
		logger.Errorf("Error to Unmarshal request body to struct")
		http.Error(w, "Unexpected end of JSON input", http.StatusBadRequest)
		return
	}

	// Read MinIo APIId & APIKey
	injectSecret(request.MinIOSetting)

	// Create MinIO Client
	client, err := minio.New(*request.MinIOSetting.EndPoint, &minio.Options{
		Creds: credentials.NewStaticV4(*request.MinIOSetting.APIId, *request.MinIOSetting.APIKey, ""),
	})
	if err != nil {
		logger.Errorf("Fail to create MinIO client")
		http.Error(w, "Fail to create client", http.StatusBadRequest)
		return
	}
	// Upload file to MinIO
	err = uploadObject(client, *request.MinIOSetting.Bucket, *request.MinIOSetting.FileName, request.RawData)
	if err != nil {
		logger.Errorf(err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

func injectSecret(setting *v1alpha1.MinIOSetting) {
	if setting == nil {
		logger.Warn("Empty MinIO service setting")
		return
	}
	if setting.Secret == nil {
		logger.Warn("Empty MinIO secret setting")
		return
	}
	secret, err := utils.GetSecret(*setting.Secret)
	if err != nil {
		logger.Errorf("Fail to get secret of %v, error: %v", *setting.Secret, err)
		return
	}
	// Load APIId & APIKey from secret
	if id, exist := secret[deviceshifubase.UsernameSecretField]; exist {
		setting.APIId = &id
	} else {
		logger.Errorf("Fail to get APIId from secret")
		return
	}
	if key, exist := secret[deviceshifubase.PasswordSecretField]; exist {
		setting.APIKey = &key
	} else {
		logger.Errorf("Fail to get APIKey from secret")
		return
	}

	logger.Infof("MinIo loaded APIId & APIKey from secret")
}

func uploadObject(client *minio.Client, bucket string, fileName string, content []byte) error {
	file, err := os.CreateTemp("", fileName)
	if err != nil {
		return errors.New("Fail to create temp file:" + err.Error())
	}
	// Close and remove temp file
	defer func(name string) {
		if file.Close() != nil {
			logger.Warn("Close MinIO temp file fail, err:", err.Error())
		}
		if os.Remove(name) != nil {
			logger.Warn("Remove MinIO temp file fail, err:" + err.Error())
		}
	}(fileName)
	// Write file content
	_, err = file.Write(content)
	if err != nil {
		return errors.New("Fail to load file content:" + err.Error())
	}
	fileStat, err := file.Stat()
	if err != nil {
		return errors.New("Fail to get file stat" + err.Error())
	}
	// Upload file to server
	_, err = client.PutObject(context.Background(),
		bucket, fileName, file, fileStat.Size(),
		minio.PutObjectOptions{ContentType: "application/octet-stream"})
	if err != nil {
		return errors.New("Upload object error:" + err.Error())
	}
	logger.Infof("Upload file success, fileName:%v", fileName)
	return nil
}
