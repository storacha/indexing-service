package aws

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	signer "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

// Taken from: https://github.com/redis/go-redis/discussions/2343

const (
	REQUEST_PROTOCOL     = "http://"
	PARAM_ACTION         = "Action"
	PARAM_USER           = "User"
	ACTION_NAME          = "connect"
	SERVICE_NAME         = "elasticache"
	PARAM_EXPIRES        = "X-Amz-Expires"
	TOKEN_EXPIRY_SECONDS = 899

	EMPTY_BODY_SHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" // the hex encoded SHA-256 of an empty string
)

type IAMAuthTokenRequest struct {
	userID    string
	cacheName string
	region    string
}

func (i *IAMAuthTokenRequest) toSignedRequestUri(ctx context.Context, credential aws.Credentials) (string, error) {
	req, err := i.getSignableRequest()
	if err != nil {
		return "", err
	}

	signedURI, _, err := i.sign(ctx, req, credential)
	if err != nil {
		return "", err
	}

	u, err := url.Parse(signedURI)
	if err != nil {
		return "", err
	}

	res := url.URL{
		Scheme:   "http",
		Host:     u.Host,
		Path:     "/",
		RawQuery: u.RawQuery,
	}

	return strings.Replace(res.String(), REQUEST_PROTOCOL, "", 1), nil
}

func (i *IAMAuthTokenRequest) getSignableRequest() (*http.Request, error) {
	query := url.Values{
		PARAM_ACTION:  {ACTION_NAME},
		PARAM_USER:    {i.userID},
		PARAM_EXPIRES: {strconv.FormatInt(int64(TOKEN_EXPIRY_SECONDS), 10)},
	}

	signURL := url.URL{
		Scheme:   "http",
		Host:     i.cacheName,
		Path:     "/",
		RawQuery: query.Encode(),
	}

	req, err := http.NewRequest(http.MethodGet, signURL.String(), nil)
	if err != nil {
		return nil, err
	}

	return req, nil
}

func (i *IAMAuthTokenRequest) sign(ctx context.Context, req *http.Request, credential aws.Credentials) (signedURI string, signedHeaders http.Header, err error) {
	s := signer.NewSigner()
	return s.PresignHTTP(ctx, credential, req, EMPTY_BODY_SHA256, SERVICE_NAME, i.region, time.Now())
}

func redisCredentialVerifier(cfg aws.Config, userID string, cacheName string) func(context.Context) (string, string, error) {
	return func(ctx context.Context) (string, string, error) {
		iamAuthTokenRequest := IAMAuthTokenRequest{
			userID:    userID,
			cacheName: cacheName,
			region:    cfg.Region,
		}

		credentials, err := cfg.Credentials.Retrieve(ctx)
		if err != nil {
			return "", "", fmt.Errorf("getting aws credentials: %w", err)
		}
		iamAuthToken, err := iamAuthTokenRequest.toSignedRequestUri(ctx, credentials)

		if err != nil {
			return "", "", fmt.Errorf("attempting to obtain signed redis loging: %w", err)
		}

		return userID, iamAuthToken, nil
	}
}
