package gowaku

import (
	"encoding/json"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/status-im/go-waku/waku/v2/node"
	"github.com/status-im/go-waku/waku/v2/protocol/pb"
	"github.com/status-im/go-waku/waku/v2/utils"
)

func wakuMessage(messageJSON string) (pb.WakuMessage, error) {
	var msg pb.WakuMessage
	err := json.Unmarshal([]byte(messageJSON), &msg)
	msg.Version = 0
	return msg, err
}

func wakuMessageSymmetricEncoding(messageJSON string, publicKey string, optionalSigningKey string) (pb.WakuMessage, error) {
	msg, err := wakuMessage(messageJSON)
	if err != nil {
		return msg, err
	}

	payload := node.Payload{
		Data: msg.Payload,
		Key: &node.KeyInfo{
			Kind: node.Asymmetric,
		},
	}

	keyBytes, err := utils.DecodeHexString(publicKey)
	if err != nil {
		return msg, err
	}

	payload.Key.PubKey, err = unmarshalPubkey(keyBytes)
	if err != nil {
		return msg, err
	}

	if optionalSigningKey != "" {
		signingKeyBytes, err := utils.DecodeHexString(optionalSigningKey)
		if err != nil {
			return msg, err
		}

		payload.Key.PrivKey, err = crypto.ToECDSA(signingKeyBytes)
		if err != nil {
			return msg, err
		}
	}

	msg.Version = 1
	msg.Payload, err = payload.Encode(1)

	return msg, err
}

func wakuMessageAsymmetricEncoding(messageJSON string, publicKey string, optionalSigningKey string) (pb.WakuMessage, error) {
	msg, err := wakuMessage(messageJSON)
	if err != nil {
		return msg, err
	}

	payload := node.Payload{
		Data: msg.Payload,
		Key: &node.KeyInfo{
			Kind: node.Asymmetric,
		},
	}

	keyBytes, err := utils.DecodeHexString(publicKey)
	if err != nil {
		return msg, err
	}

	payload.Key.PubKey, err = unmarshalPubkey(keyBytes)
	if err != nil {
		return msg, err
	}

	if optionalSigningKey != "" {
		signingKeyBytes, err := utils.DecodeHexString(optionalSigningKey)
		if err != nil {
			return msg, err
		}

		payload.Key.PrivKey, err = crypto.ToECDSA(signingKeyBytes)
		if err != nil {
			return msg, err
		}
	}

	msg.Version = 1
	msg.Payload, err = payload.Encode(1)

	return msg, err
}
