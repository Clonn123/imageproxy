// Copyright 2013 The imageproxy authors.
// SPDX-License-Identifier: Apache-2.0

package imageproxy

import (
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

const defaultStoragePresignExpiry = 15 * time.Minute

// PrefixStorage describes how a storage selected by the first request path
// segment should be resolved.
type PrefixStorage struct {
	BaseURL *url.URL
	S3      *S3Storage
}

// S3Storage describes an S3-compatible origin storage.
type S3Storage struct {
	Endpoint       *url.URL
	Region         string
	Bucket         string
	Prefix         string
	AccessKey      string
	SecretKey      string
	DisableSSL     bool
	ForcePathStyle bool
	PresignExpiry  time.Duration
	SessionToken   string
}

// PresignedGetURL returns a stable logical URL and a signed fetch URL for the
// specified object path.
func (s *S3Storage) PresignedGetURL(objectPath string) (_ *url.URL, _ *url.URL, err error) {
	key := strings.TrimPrefix(objectPath, "/")
	key = path.Join(strings.Trim(s.Prefix, "/"), key)

	config := aws.NewConfig().
		WithRegion(s.region()).
		WithCredentials(credentials.NewStaticCredentials(s.AccessKey, s.SecretKey, s.SessionToken)).
		WithS3ForcePathStyle(s.ForcePathStyle)

	if s.Endpoint != nil {
		config = config.WithEndpoint(s.Endpoint.String())
	}
	if s.DisableSSL {
		config = config.WithDisableSSL(true)
	}

	sess, err := session.NewSession(config)
	if err != nil {
		return nil, nil, err
	}

	input := &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	}
	req, _ := s3.New(sess).GetObjectRequest(input)

	signedURL, err := req.Presign(s.presignExpiry())
	if err != nil {
		return nil, nil, err
	}

	fetchURL, err := url.Parse(signedURL)
	if err != nil {
		return nil, nil, err
	}

	logicalURL := *fetchURL
	logicalURL.RawQuery = ""
	logicalURL.ForceQuery = false
	logicalURL.Fragment = ""

	return &logicalURL, fetchURL, nil
}

func (s *S3Storage) region() string {
	if s.Region != "" {
		return s.Region
	}
	return "us-east-1"
}

func (s *S3Storage) presignExpiry() time.Duration {
	if s.PresignExpiry > 0 {
		return s.PresignExpiry
	}
	return defaultStoragePresignExpiry
}

func decodeStorageObjectPath(rawPath string) string {
	decoded, err := url.PathUnescape(strings.TrimPrefix(rawPath, "/"))
	if err != nil {
		return strings.TrimPrefix(rawPath, "/")
	}
	return decoded
}

func (s *S3Storage) Validate() error {
	switch {
	case s.Bucket == "":
		return fmt.Errorf("bucket cannot be empty")
	case s.AccessKey == "":
		return fmt.Errorf("accessKey cannot be empty")
	case s.SecretKey == "":
		return fmt.Errorf("secretKey cannot be empty")
	case s.Endpoint != nil && !s.Endpoint.IsAbs():
		return fmt.Errorf("endpoint must be absolute")
	case s.Endpoint != nil && s.Endpoint.Scheme != "http" && s.Endpoint.Scheme != "https":
		return fmt.Errorf("endpoint must use http or https")
	}

	return nil
}
