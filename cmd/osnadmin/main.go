/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric/internal/osnadmin"
	"github.com/hyperledger/fabric/internal/pkg/comm"
	"github.com/hyperledger/fabric/protoutil"
	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	kingpin.Version("0.0.1")

	output, exit, err := executeForArgs(os.Args[1:])
	if err != nil {
		kingpin.Fatalf("parsing arguments: %s. Try --help", err)
	}
	fmt.Println(output)
	os.Exit(exit)
}

func executeForArgs(args []string) (output string, exit int, err error) {
	//
	// command line flags
	//
	app := kingpin.New("osnadmin", "Orderer Service Node (OSN) administration")
	orderer := app.Flag("orderer-address", "Endpoint of the OSN").Short('o').Required().String()
	caFile := app.Flag("ca-file", "Path to file containing PEM-encoded trusted certificate(s) for the OSN").Required().String()
	clientCert := app.Flag("client-cert", "Path to file containing PEM-encoded X509 public key to use for mutual TLS communication with the OSN").Required().String()
	clientKey := app.Flag("client-key", "Path to file containing PEM-encoded private key to use for mutual TLS communication with the OSN").Required().String()

	channel := app.Command("channel", "Channel actions")

	join := channel.Command("join", "Join an Ordering Service Node (OSN) to a channel. If the channel does not yet exist, it will be created.")
	joinChannelID := join.Flag("channel-id", "Channel ID").Short('c').Required().String()
	configBlockPath := join.Flag("config-block", "Path to the file containing the config block").Short('b').Required().String()

	list := channel.Command("list", "List channel information for an Ordering Service Node (OSN). If the channel-id flag is set, more detailed information will be provided for that channel.")
	listChannelID := list.Flag("channel-id", "Channel ID").Short('c').String()

	remove := channel.Command("remove", "Remove an Ordering Service Node (OSN) from a channel.")
	removeChannelID := remove.Flag("channel-id", "Channel ID").Short('c').Required().String()

	command := kingpin.MustParse(app.Parse(args))

	//
	// flag validation
	//
	osnURL := fmt.Sprintf("https://%s", *orderer)

	caCertPool := x509.NewCertPool()
	caFilePEM, err := ioutil.ReadFile(*caFile)
	if err != nil {
		return "", 1, fmt.Errorf("reading orderer CA certificate: %s", err)
	}
	err = comm.AddPemToCertPool(caFilePEM, caCertPool)
	if err != nil {
		return "", 1, fmt.Errorf("adding ca-file PEM to cert pool: %s", err)
	}

	tlsClientCert, err := tls.LoadX509KeyPair(*clientCert, *clientKey)
	if err != nil {
		return "", 1, fmt.Errorf("loading client cert/key pair: %s", err)
	}

	var marshaledConfigBlock []byte
	if *configBlockPath != "" {
		marshaledConfigBlock, err = ioutil.ReadFile(*configBlockPath)
		if err != nil {
			return "", 1, fmt.Errorf("reading config block: %s", err)
		}

		err = validateBlockChannelID(marshaledConfigBlock, *joinChannelID)
		if err != nil {
			return "", 1, err
		}
	}

	//
	// call the underlying implementations
	//
	var resp *http.Response

	switch command {
	case join.FullCommand():
		resp, err = osnadmin.Join(osnURL, marshaledConfigBlock, caCertPool, tlsClientCert)
	case list.FullCommand():
		if *listChannelID != "" {
			resp, err = osnadmin.ListSingleChannel(osnURL, *listChannelID, caCertPool, tlsClientCert)
			break
		}
		resp, err = osnadmin.ListAllChannels(osnURL, caCertPool, tlsClientCert)
	case remove.FullCommand():
		resp, err = osnadmin.Remove(osnURL, *removeChannelID, caCertPool, tlsClientCert)
	}
	if err != nil {
		return errorOutput(err), 1, nil
	}

	bodyBytes, err := readBodyBytes(resp.Body)
	if err != nil {
		return errorOutput(err), 1, nil
	}

	return responseOutput(resp.StatusCode, bodyBytes), 0, nil
}

func responseOutput(statusCode int, responseBody []byte) string {
	status := fmt.Sprintf("Status: %d", statusCode)

	var buffer bytes.Buffer
	json.Indent(&buffer, responseBody, "", "\t")
	response := fmt.Sprintf("%s", buffer.Bytes())

	output := fmt.Sprintf("%s\n%s", status, response)

	return output
}

func readBodyBytes(body io.ReadCloser) ([]byte, error) {
	bodyBytes, err := ioutil.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("reading http response body: %s", err)
	}
	body.Close()

	return bodyBytes, nil
}

func errorOutput(err error) string {
	return fmt.Sprintf("Error: %s\n", err)
}

func validateBlockChannelID(blockBytes []byte, channelID string) error {
	block := &common.Block{}
	err := proto.Unmarshal(blockBytes, block)
	if err != nil {
		return fmt.Errorf("unmarshaling block: %s", err)
	}

	blockChannelID, err := protoutil.GetChannelIDFromBlock(block)
	if err != nil {
		return err
	}

	// quick sanity check that the orderer admin is joining
	// the channel they think they're joining.
	if channelID != blockChannelID {
		return fmt.Errorf("specified --channel-id %s does not match channel ID %s in config block", channelID, blockChannelID)
	}

	return nil
}
