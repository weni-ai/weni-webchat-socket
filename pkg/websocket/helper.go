package websocket

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/ilhasoft/wwcs/config"
)

const errorPrefix = "invalid payload:"

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
				log.Error(ErrorDecodingMedia, err.Error())
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
		Context: payload.Context,
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

var (
	s3session     *session.Session
	s3sessionOnce sync.Once
)

// S3Session returns the lazily initialized AWS S3 session
func S3Session() *session.Session {
	s3sessionOnce.Do(func() {
		s3session = connectAWS()
	})
	return s3session
}

func connectAWS() *session.Session {
	cfg := config.Get().S3
	sess, err := session.NewSession(
		&aws.Config{
			Credentials:      credentials.NewStaticCredentials(cfg.AccessKey, cfg.SecretKey, ""),
			Endpoint:         aws.String(cfg.Endpoint),
			Region:           aws.String(cfg.Region),
			S3ForcePathStyle: aws.Bool(cfg.ForcePathStyle),
			DisableSSL:       aws.Bool(cfg.DisableSSL),
		},
	)
	if err != nil {
		log.Panic(err)
	}
	return sess
}

// test the login can access a typical aws service (s3) and known bucket
func CheckAWS() error {
	cfg := config.Get().S3
	svc := s3.New(S3Session())
	params := &s3.ListObjectsInput{
		Bucket: aws.String(cfg.Bucket),
	}
	_, err := svc.ListObjects(params)
	if err != nil {
		log.Error(err)
		return err
	}

	return nil
}

// TODO: Mock and test it
func uploadToS3(from string, file io.Reader, fileType string) (string, error) {
	cfg := config.Get().S3
	uploader := s3manager.NewUploader(S3Session())

	key := fmt.Sprintf("%s-%d.%s", from, time.Now().Unix(), fileType)
	_, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(cfg.Bucket),
		Key:    aws.String(key),
		Body:   file,
	})
	if err != nil {
		log.Error(err)
		return "", err
	}

	url := fmt.Sprintf("https://%s.s3.amazonaws.com/%s", cfg.Bucket, key)
	return url, nil
}

func CheckRedis(app *App) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	err := app.RDB.Ping(ctx).Err()
	if err != nil {
		log.Error(err)
	}
	return err
}

func CheckDB(app *App) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	conn := app.MDB.Client()
	if err := conn.Ping(ctx, nil); err != nil {
		log.Error("fail to ping MongoDB", err.Error())
		return err
	}
	return nil
}
