/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go/common"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/bccsp/pkcs11"
	"github.com/hyperledger/fabric/bccsp/sw"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/flogging"
	"github.com/hyperledger/fabric/common/util"
	"github.com/hyperledger/fabric/core/config/configtest"
	"github.com/hyperledger/fabric/internal/configtxgen/encoder"
	"github.com/hyperledger/fabric/internal/configtxgen/genesisconfig"
	"github.com/hyperledger/fabric/internal/peer/common"
	"github.com/hyperledger/fabric/msp"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestInitConfig(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()

	type args struct {
		cmdRoot string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "Empty command root",
			args:    args{cmdRoot: ""},
			wantErr: true,
		},
		{
			name:    "Bad command root",
			args:    args{cmdRoot: "cre"},
			wantErr: true,
		},
		{
			name:    "Good command root",
			args:    args{cmdRoot: "core"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := common.InitConfig(tt.args.cmdRoot); (err != nil) != tt.wantErr {
				t.Errorf("InitConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestInitCryptoMissingDir(t *testing.T) {
	dir := path.Join(os.TempDir(), util.GenerateUUID())
	err := common.InitCrypto(dir, "SampleOrg", msp.ProviderTypeToString(msp.FABRIC))
	require.Error(t, err, "Should not be able to initialize crypto with non-existing directory")
	require.Contains(t, err.Error(), fmt.Sprintf("specified path \"%s\" does not exist", dir))
}

func TestInitCryptoFileNotDir(t *testing.T) {
	file := path.Join(os.TempDir(), util.GenerateUUID())
	err := ioutil.WriteFile(file, []byte{}, 0644)
	require.Nil(t, err, "Failed to create test file")
	defer os.Remove(file)
	err = common.InitCrypto(file, "SampleOrg", msp.ProviderTypeToString(msp.FABRIC))
	require.Error(t, err, "Should not be able to initialize crypto with a file instead of a directory")
	require.Contains(t, err.Error(), fmt.Sprintf("specified path \"%s\" is not a directory", file))
}

func TestInitCrypto(t *testing.T) {
	mspConfigPath := configtest.GetDevMspDir()
	localMspId := "SampleOrg"
	err := common.InitCrypto(mspConfigPath, localMspId, msp.ProviderTypeToString(msp.FABRIC))
	require.NoError(t, err, "Unexpected error [%s] calling InitCrypto()", err)
	localMspId = ""
	err = common.InitCrypto(mspConfigPath, localMspId, msp.ProviderTypeToString(msp.FABRIC))
	require.Error(t, err, fmt.Sprintf("Expected error [%s] calling InitCrypto()", err))
}

func TestSetBCCSPKeystorePath(t *testing.T) {
	cfgKey := "peer.BCCSP.SW.FileKeyStore.KeyStore"
	cfgPath := "./testdata"
	absPath, err := filepath.Abs(cfgPath)
	require.NoError(t, err)

	keystorePath := "/msp/keystore"
	defer os.Unsetenv("FABRIC_CFG_PATH")

	os.Setenv("FABRIC_CFG_PATH", cfgPath)
	viper.Reset()
	err = common.InitConfig("notset")
	common.SetBCCSPKeystorePath()
	t.Log(viper.GetString(cfgKey))
	require.Equal(t, "", viper.GetString(cfgKey))
	require.Nil(t, viper.Get(cfgKey))

	viper.Reset()
	err = common.InitConfig("absolute")
	require.NoError(t, err)
	common.SetBCCSPKeystorePath()
	t.Log(viper.GetString(cfgKey))
	require.Equal(t, keystorePath, viper.GetString(cfgKey))

	viper.Reset()
	err = common.InitConfig("relative")
	require.NoError(t, err)
	common.SetBCCSPKeystorePath()
	t.Log(viper.GetString(cfgKey))
	require.Equal(t, filepath.Join(absPath, keystorePath), viper.GetString(cfgKey))

	viper.Reset()
}

func TestCheckLogLevel(t *testing.T) {
	type args struct {
		level string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "Empty level",
			args:    args{level: ""},
			wantErr: true,
		},
		{
			name:    "Valid level",
			args:    args{level: "warning"},
			wantErr: false,
		},
		{
			name:    "Invalid level",
			args:    args{level: "foobaz"},
			wantErr: true,
		},
		{
			name:    "Valid level",
			args:    args{level: "error"},
			wantErr: false,
		},
		{
			name:    "Valid level",
			args:    args{level: "info"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := common.CheckLogLevel(tt.args.level); (err != nil) != tt.wantErr {
				t.Errorf("CheckLogLevel() args = %v error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
		})
	}
}

func TestGetDefaultSigner(t *testing.T) {
	tests := []struct {
		name    string
		want    msp.SigningIdentity
		wantErr bool
	}{
		{
			name:    "Should return DefaultSigningIdentity",
			want:    nil,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := common.GetDefaultSigner()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetDefaultSigner() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestInitCmd(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()
	defer viper.Reset()

	// test that InitCmd doesn't remove existing loggers from the logger levels map
	flogging.MustGetLogger("test")
	flogging.ActivateSpec("test=error")
	require.Equal(t, "error", flogging.LoggerLevel("test"))
	flogging.MustGetLogger("chaincode")
	require.Equal(t, flogging.DefaultLevel(), flogging.LoggerLevel("chaincode"))
	flogging.MustGetLogger("test.test2")
	flogging.ActivateSpec("test.test2=warn")
	require.Equal(t, "warn", flogging.LoggerLevel("test.test2"))

	origEnvValue := os.Getenv("FABRIC_LOGGING_SPEC")
	os.Setenv("FABRIC_LOGGING_SPEC", "chaincode=debug:test.test2=fatal:abc=error")
	common.InitCmd(&cobra.Command{}, nil)
	require.Equal(t, "debug", flogging.LoggerLevel("chaincode"))
	require.Equal(t, "info", flogging.LoggerLevel("test"))
	require.Equal(t, "fatal", flogging.LoggerLevel("test.test2"))
	require.Equal(t, "error", flogging.LoggerLevel("abc"))
	os.Setenv("FABRIC_LOGGING_SPEC", origEnvValue)
}

func TestInitCmdWithoutInitCrypto(t *testing.T) {
	cleanup := configtest.SetDevFabricConfigPath(t)
	defer cleanup()
	defer viper.Reset()

	peerCmd := &cobra.Command{
		Use: "peer",
	}
	lifecycleCmd := &cobra.Command{
		Use: "lifecycle",
	}
	chaincodeCmd := &cobra.Command{
		Use: "chaincode",
	}
	packageCmd := &cobra.Command{
		Use: "package",
	}
	// peer lifecycle chaincode package
	chaincodeCmd.AddCommand(packageCmd)
	lifecycleCmd.AddCommand(chaincodeCmd)
	peerCmd.AddCommand(lifecycleCmd)

	// MSPCONFIGPATH is default value
	common.InitCmd(packageCmd, nil)

	// set MSPCONFIGPATH to be a missing dir, the function InitCrypto will fail
	// confirm that 'peer lifecycle chaincode package' mandates does not require MSPCONFIG information
	viper.SetEnvPrefix("core")
	viper.AutomaticEnv()
	replacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(replacer)
	dir := os.TempDir() + "/" + util.GenerateUUID()
	os.Setenv("CORE_PEER_MSPCONFIGPATH", dir)

	common.InitCmd(packageCmd, nil)
}

func TestSetBCCSPConfigOverrides(t *testing.T) {
	bccspConfig := factory.GetDefaultOpts()
	envConfig := &factory.FactoryOpts{
		Default: "test-default",
		SW: &factory.SwOpts{
			Hash:     "SHA2",
			Security: 256,
		},
		PKCS11: &pkcs11.PKCS11Opts{
			Hash:     "test-pkcs11-hash",
			Security: 12345,
			Library:  "test-pkcs11-library",
			Label:    "test-pkcs11-label",
			Pin:      "test-pkcs11-pin",
		},
	}

	t.Run("success", func(t *testing.T) {
		cleanup := setBCCSPEnvVariables(envConfig)
		defer cleanup()
		err := common.SetBCCSPConfigOverrides(bccspConfig)
		require.NoError(t, err)
		require.Equal(t, envConfig, bccspConfig)
	})

	t.Run("PKCS11 security set to string value", func(t *testing.T) {
		cleanup := setBCCSPEnvVariables(envConfig)
		defer cleanup()
		os.Setenv("CORE_PEER_BCCSP_PKCS11_SECURITY", "INSECURITY")

		err := common.SetBCCSPConfigOverrides(bccspConfig)
		require.EqualError(t, err, "CORE_PEER_BCCSP_PKCS11_SECURITY set to non-integer value: INSECURITY")
	})
}

func TestGetOrdererEndpointFromConfigTx(t *testing.T) {
	require.NoError(t, msptesttools.LoadMSPSetupForTesting())
	signer, err := common.GetDefaultSigner()
	require.NoError(t, err)
	factory.InitFactories(nil)
	cryptoProvider, err := sw.NewDefaultSecurityLevelWithKeystore(sw.NewDummyKeyStore())
	require.NoError(t, err)

	t.Run("green-path", func(t *testing.T) {
		profile := genesisconfig.Load(genesisconfig.SampleInsecureSoloProfile, configtest.GetDevConfigDir())
		channelGroup, err := encoder.NewChannelGroup(profile)
		require.NoError(t, err)
		channelConfig := &cb.Config{ChannelGroup: channelGroup}

		ordererAddresses := channelconfig.OrdererAddressesValue([]string{"order-1-endpoint", "order-2-end-point"})
		channelConfig.ChannelGroup.Values[ordererAddresses.Key()] = &cb.ConfigValue{
			Value: protoutil.MarshalOrPanic(ordererAddresses.Value()),
		}

		mockEndorserClient := common.GetMockEndorserClient(
			&pb.ProposalResponse{
				Response:    &pb.Response{Status: 200, Payload: protoutil.MarshalOrPanic(channelConfig)},
				Endorsement: &pb.Endorsement{},
			},
			nil,
		)

		ordererEndpoints, err := common.GetOrdererEndpointOfChain("test-channel", signer, mockEndorserClient, cryptoProvider)
		require.NoError(t, err)
		require.Equal(t, []string{"order-1-endpoint", "order-2-end-point"}, ordererEndpoints)
	})

	t.Run("error-invoking-CSCC", func(t *testing.T) {
		mockEndorserClient := common.GetMockEndorserClient(
			nil,
			errors.Errorf("cscc-invocation-error"),
		)
		_, err := common.GetOrdererEndpointOfChain("test-channel", signer, mockEndorserClient, cryptoProvider)
		require.EqualError(t, err, "error endorsing GetChannelConfig: cscc-invocation-error")
	})

	t.Run("nil-response", func(t *testing.T) {
		mockEndorserClient := common.GetMockEndorserClient(
			nil,
			nil,
		)
		_, err := common.GetOrdererEndpointOfChain("test-channel", signer, mockEndorserClient, cryptoProvider)
		require.EqualError(t, err, "received nil proposal response")
	})

	t.Run("bad-status-code-from-cscc", func(t *testing.T) {
		mockEndorserClient := common.GetMockEndorserClient(
			&pb.ProposalResponse{
				Response:    &pb.Response{Status: 404, Payload: []byte{}},
				Endorsement: &pb.Endorsement{},
			},
			nil,
		)
		_, err := common.GetOrdererEndpointOfChain("test-channel", signer, mockEndorserClient, cryptoProvider)
		require.EqualError(t, err, "error bad proposal response 404: ")
	})

	t.Run("unmarshalable-config", func(t *testing.T) {
		mockEndorserClient := common.GetMockEndorserClient(
			&pb.ProposalResponse{
				Response:    &pb.Response{Status: 200, Payload: []byte("unmarshalable-config")},
				Endorsement: &pb.Endorsement{},
			},
			nil,
		)
		_, err := common.GetOrdererEndpointOfChain("test-channel", signer, mockEndorserClient, cryptoProvider)
		require.EqualError(t, err, "error unmarshaling channel config: unexpected EOF")
	})

	t.Run("unloadable-config", func(t *testing.T) {
		mockEndorserClient := common.GetMockEndorserClient(
			&pb.ProposalResponse{
				Response:    &pb.Response{Status: 200, Payload: []byte{}},
				Endorsement: &pb.Endorsement{},
			},
			nil,
		)
		_, err := common.GetOrdererEndpointOfChain("test-channel", signer, mockEndorserClient, cryptoProvider)
		require.EqualError(t, err, "error loading channel config: config must contain a channel group")
	})
}

func setBCCSPEnvVariables(bccspConfig *factory.FactoryOpts) (cleanup func()) {
	os.Setenv("CORE_PEER_BCCSP_DEFAULT", bccspConfig.Default)
	os.Setenv("CORE_PEER_BCCSP_SW_SECURITY", strconv.Itoa(bccspConfig.SW.Security))
	os.Setenv("CORE_PEER_BCCSP_SW_HASH", bccspConfig.SW.Hash)
	os.Setenv("CORE_PEER_BCCSP_PKCS11_SECURITY", strconv.Itoa(bccspConfig.PKCS11.Security))
	os.Setenv("CORE_PEER_BCCSP_PKCS11_HASH", bccspConfig.PKCS11.Hash)
	os.Setenv("CORE_PEER_BCCSP_PKCS11_PIN", bccspConfig.PKCS11.Pin)
	os.Setenv("CORE_PEER_BCCSP_PKCS11_LABEL", bccspConfig.PKCS11.Label)
	os.Setenv("CORE_PEER_BCCSP_PKCS11_LIBRARY", bccspConfig.PKCS11.Library)

	return func() {
		os.Unsetenv("CORE_PEER_BCCSP_DEFAULT")
		os.Unsetenv("CORE_PEER_BCCSP_SW_SECURITY")
		os.Unsetenv("CORE_PEER_BCCSP_SW_HASH")
		os.Unsetenv("CORE_PEER_BCCSP_PKCS11_SECURITY")
		os.Unsetenv("CORE_PEER_BCCSP_PKCS11_HASH")
		os.Unsetenv("CORE_PEER_BCCSP_PKCS11_PIN")
		os.Unsetenv("CORE_PEER_BCCSP_PKCS11_LABEL")
		os.Unsetenv("CORE_PEER_BCCSP_PKCS11_LIBRARY")
	}
}
