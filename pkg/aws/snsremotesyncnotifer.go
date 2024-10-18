package aws

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipld/go-ipld-prime"
)

var log = logging.Logger("aws")

type SNSRemoteSyncMessage struct {
	Head string `json:"Head,omitempty"`
	Prev string `json:"Prev,omitempty"`
}

type SNSRemoteSyncNotifier struct {
	topicArn  string
	snsClient *sns.Client
}

func NewSNSRemoteSyncNotifier(config aws.Config, topicArn string) *SNSRemoteSyncNotifier {
	return &SNSRemoteSyncNotifier{
		snsClient: sns.NewFromConfig(config),
		topicArn:  topicArn,
	}
}
func (s *SNSRemoteSyncNotifier) NotifyRemoteSync(ctx context.Context, head, prev ipld.Link) {
	messageJSON, err := json.Marshal(SNSRemoteSyncMessage{
		Head: head.String(),
		Prev: prev.String(),
	})
	if err != nil {
		log.Errorf("serializing remote sync message: %s", err.Error())
	}
	_, err = s.snsClient.Publish(ctx, &sns.PublishInput{TopicArn: aws.String(s.topicArn), Message: aws.String(string(messageJSON))})
	if err != nil {
		log.Errorf("serializing remote sync message: %s", err.Error())
	}
}
