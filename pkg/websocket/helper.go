package websocket

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/ilhasoft/wwcs/config"
)

const errorPrefix = "invalid payload:"

const MSG_EXPIRATION = (2 * 24) * time.Hour

// validate errors
var (
	ErrorInvalidPayloadType = fmt.Errorf("%s invalid payload type", errorPrefix)
	// message
	ErrorBlankFrom          = fmt.Errorf("%s blank message from", errorPrefix)
	ErrorBlankMessageType   = fmt.Errorf("%s blank message type", errorPrefix)
	ErrorInvalidMessageType = fmt.Errorf("%s invalid message type", errorPrefix)
	ErrorDecodingMedia      = fmt.Errorf("%s could not decode media", errorPrefix)
	ErrorUploadingToS3      = fmt.Errorf("%s can not upload image to s3", errorPrefix)
	// close_session
	ErrorInvalidToken  = fmt.Errorf("token does not match that of the client")
	ErrorInvalidClient = fmt.Errorf("Client not found")
)

func formatOutgoingPayload(payload OutgoingPayload) (OutgoingPayload, error) {
	message := payload.Message
	var logs []string

	// check if payload type is message
	if payload.Type != "message" && payload.Type != "ping" {
		return OutgoingPayload{}, ErrorInvalidPayloadType
	}
	// check if from is blank
	if payload.From == "" {
		return OutgoingPayload{}, ErrorBlankFrom
	}
	// check if type is blank
	if payload.Type != "ping" && message.Type == "" {
		return OutgoingPayload{}, ErrorBlankMessageType
	}

	if message.Media != "" {
		if message.Type == "image" || message.Type == "video" || message.Type == "audio" || message.Type == "file" {
			var err error

			// need to remove data:[<MIME-type>][;charset=<encoding>][;base64]` from the beginning
			base64String := message.Media[strings.IndexByte(message.Media, ',')+1:]
			base64Media, err := base64.StdEncoding.DecodeString(base64String)
			if err != nil {
				return OutgoingPayload{}, ErrorDecodingMedia
			}
			// get the fileType in data:[<MIME-type>][;charset=<encoding>][;base64]` at <MIME-type>
			fileType := message.Media[strings.IndexByte(message.Media, '/')+1 : strings.IndexByte(message.Media, ';')]

			message.MediaURL, err = uploadToS3(payload.From, bytes.NewBuffer(base64Media), fileType)
			if err != nil {
				return OutgoingPayload{}, ErrorUploadingToS3
			}
		}
	}

	presenter := OutgoingPayload{
		Type: payload.Type,
		From: payload.From,
		Message: Message{
			Type:      message.Type,
			Timestamp: fmt.Sprint(time.Now().Unix()),
		},
	}
	// validate all message types
	if message.Type == "text" {
		if message.Text == "" {
			logs = append(logs, "blank text")
		}
		presenter.Message.Text = message.Text
	} else if message.Type == "image" {
		if message.MediaURL == "" {
			logs = append(logs, "blank media_url")
		}
		presenter.Message.MediaURL = message.MediaURL
		presenter.Message.Caption = message.Caption
	} else if message.Type == "video" {
		if message.MediaURL == "" {
			logs = append(logs, "blank media_url")
		}
		presenter.Message.MediaURL = message.MediaURL
		presenter.Message.Caption = message.Caption
	} else if message.Type == "audio" {
		if message.MediaURL == "" {
			logs = append(logs, "blank media_url")
		}
		presenter.Message.MediaURL = message.MediaURL
		presenter.Message.Caption = message.Caption
	} else if message.Type == "file" {
		if message.MediaURL == "" {
			logs = append(logs, "blank media_url")
		}
		presenter.Message.MediaURL = message.MediaURL
		presenter.Message.Caption = message.Caption
	} else if message.Type == "location" {
		if message.Latitude == "" {
			logs = append(logs, "blank latitude")
		}
		if message.Longitude == "" {
			logs = append(logs, "blank longitude")
		}
		presenter.Message.Latitude = message.Latitude
		presenter.Message.Longitude = message.Longitude
	} else if payload.Type == "ping" {
		presenter.Message.Type = "pong"
	} else {
		return OutgoingPayload{}, ErrorInvalidMessageType
	}

	// append all logs to one error
	if logs != nil {
		return OutgoingPayload{}, fmt.Errorf("%s %s", errorPrefix, strings.Join(logs, ", "))
	}

	return presenter, nil
}

func validateOutgoingPayloadRegister(payload OutgoingPayload) error {
	var logs []string
	if payload.From == "" {
		logs = append(logs, "blank from")
	}

	if payload.Callback == "" {
		logs = append(logs, "blank callback")
	}

	if logs != nil {
		return fmt.Errorf("%s %s", errorPrefix, strings.Join(logs, ", "))
	}

	return nil
}

var S3session = connectAWS()

func connectAWS() *session.Session {
	config := config.Get.S3
	S3session, err := session.NewSession(
		&aws.Config{
			Credentials:      credentials.NewStaticCredentials(config.AccessKey, config.SecretKey, ""),
			Endpoint:         aws.String(config.Endpoint),
			Region:           aws.String(config.Region),
			S3ForcePathStyle: aws.Bool(config.ForcePathStyle),
			DisableSSL:       aws.Bool(config.DisableSSL),
		},
	)
	if err != nil {
		log.Panic(err)
	}
	return S3session
}

// test the login can access a typical aws service (s3) and known bucket
func CheckAWS() error {
	config := config.Get.S3
	svc := s3.New(S3session)
	params := &s3.ListObjectsInput{
		Bucket: aws.String(config.Bucket),
	}
	_, err := svc.ListObjects(params)

	return err
}

// TODO: Mock and test it
func uploadToS3(from string, file io.Reader, fileType string) (string, error) {
	config := config.Get.S3
	uploader := s3manager.NewUploader(S3session)

	key := fmt.Sprintf("%s-%d.%s", from, time.Now().UnixNano(), fileType)
	_, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(config.Bucket),
		Key:    aws.String(key),
		Body:   file,
	})
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://%s.s3-%s.amazonaws.com/%s", config.Bucket, config.Region, key)
	return url, nil
}
