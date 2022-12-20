package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/waku-org/go-waku/waku/v2/node"
	"github.com/waku-org/go-waku/waku/v2/protocol/pb"
	"github.com/waku-org/go-waku/waku/v2/protocol/store"
)

var log = logging.Logger("basic2")

func main() {
	lvl, err := logging.LevelFromString("error")
	if err != nil {
		panic(err)
	}
	logging.SetAllLoggers(lvl)

	hostAddr, _ := net.ResolveTCPAddr("tcp", fmt.Sprint("0.0.0.0:0"))

	key, err := randomHex(32)
	if err != nil {
		log.Error("Could not generate random key")
		return
	}
	prvKey, err := crypto.HexToECDSA(key)
	if err != nil {
		log.Error(err)
		return
	}

	ctx := context.Background()

	wakuNode, err := node.New(ctx,
		node.WithPrivateKey(prvKey),
		node.WithHostAddress(hostAddr),
		node.WithNTP(),
		node.WithWakuRelay(),
	)
	if err != nil {
		log.Error(err)
		return
	}

	if err := wakuNode.Start(); err != nil {
		log.Error(err)
		return
	}

	nodes := []string{
		"/dns4/node-01.ac-cn-hongkong-c.status.prod.statusim.net/tcp/30303/p2p/16Uiu2HAkvEZgh3KLwhLwXg95e5ojM8XykJ4Kxi2T7hk22rnA7pJC",
		"/dns4/node-01.do-ams3.status.prod.statusim.net/tcp/30303/p2p/16Uiu2HAm6HZZr7aToTvEBPpiys4UxajCTU97zj5v7RNR2gbniy1D",
		"/dns4/node-01.gc-us-central1-a.status.prod.statusim.net/tcp/30303/p2p/16Uiu2HAkwBp8T6G77kQXSNMnxgaMky1JeyML5yqoTHRM8dbeCBNb",
		"/dns4/node-02.ac-cn-hongkong-c.status.prod.statusim.net/tcp/30303/p2p/16Uiu2HAmFy8BrJhCEmCYrUfBdSNkrPw6VHExtv4rRp1DSBnCPgx8",
		"/dns4/node-02.do-ams3.status.prod.statusim.net/tcp/30303/p2p/16Uiu2HAmSve7tR5YZugpskMv2dmJAsMUKmfWYEKRXNUxRaTCnsXV",
		"/dns4/node-02.gc-us-central1-a.status.prod.statusim.net/tcp/30303/p2p/16Uiu2HAmDQugwDHM3YeUp86iGjrUvbdw3JPRgikC7YoGBsT2ymMg",
	}
	peerIDs := []string{
		"16Uiu2HAkvEZgh3KLwhLwXg95e5ojM8XykJ4Kxi2T7hk22rnA7pJC",
		"16Uiu2HAm6HZZr7aToTvEBPpiys4UxajCTU97zj5v7RNR2gbniy1D",
		"16Uiu2HAkwBp8T6G77kQXSNMnxgaMky1JeyML5yqoTHRM8dbeCBNb",
		"16Uiu2HAmFy8BrJhCEmCYrUfBdSNkrPw6VHExtv4rRp1DSBnCPgx8",
		"16Uiu2HAmSve7tR5YZugpskMv2dmJAsMUKmfWYEKRXNUxRaTCnsXV",
		"16Uiu2HAmDQugwDHM3YeUp86iGjrUvbdw3JPRgikC7YoGBsT2ymMg",
	}

	fmt.Println("Connecting to nodes....")
	for _, n := range nodes {
		err := wakuNode.DialPeer(context.Background(), n)
		if err != nil {
			fmt.Println("Could not connect to ", n, err)
		}
	}

	fmt.Println("Waiting for mesh to form....")
	time.Sleep(3 * time.Second) // Wait some time for mesh to form

	contentTopic, err := randomHex(20)
	if err != nil {
		panic(err.Error())
	}

	msg := &pb.WakuMessage{
		Payload:      []byte{0x01, 0x02, 0x03},
		Timestamp:    wakuNode.Timesource().Now().UnixNano(),
		ContentTopic: contentTopic,
		Version:      0,
	}

	msgId, err := wakuNode.Relay().Publish(context.Background(), msg)
	if err != nil {
		panic(err.Error())
	}

	fmt.Println("THE MESSAGE ID: ", hex.EncodeToString(msgId))

	time.Sleep(3 * time.Second) // Wait some time for storenodes to insert message

	for _, p := range peerIDs {

		peerID, _ := peer.Decode(p)

		result, err := wakuNode.Store().Query(context.Background(), store.Query{
			ContentTopics: []string{contentTopic},
			StartTime:     wakuNode.Timesource().Now().UnixNano() - int64(30*time.Second),
		}, store.WithPeer(peerID))
		if err != nil {
			fmt.Println("STORE ERROR", peerID, err)
			continue
		}

		if len(result.Messages) != 1 {
			fmt.Println("NO MESSAGES", peerID, err)
			continue
		}

		msgHash, _, _ := result.Messages[0].Hash()
		fmt.Println("MessageID: ", hex.EncodeToString(msgHash), " on peer: ", peerID)
	}

	// Wait for a SIGINT or SIGTERM signal
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	fmt.Println("\n\n\nReceived signal, shutting down...")

	// shut the node down
	wakuNode.Stop()

}

func randomHex(n int) (string, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
