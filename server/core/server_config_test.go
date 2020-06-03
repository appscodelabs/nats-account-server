/*
 * Copyright 2019 The NATS Authors
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package core

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats-account-server/server/conf"
	nats "github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	nsc "github.com/nats-io/nsc/cmd/store"
	"github.com/stretchr/testify/require"
)

func TestStartWithDirFlag(t *testing.T) {
	path, err := ioutil.TempDir(os.TempDir(), "store")
	require.NoError(t, err)

	flags := Flags{
		Debug:     true,
		Verbose:   true,
		Directory: path,
	}

	server := NewAccountServer()
	server.InitializeFromFlags(flags)
	server.config.Logging.Custom = NewNilLogger()
	server.config.HTTP.Port = 0 // reset port so we don't conflict
	err = server.Start()
	require.NoError(t, err)
	defer server.Stop()

	require.NotNil(t, server.Logger())

	httpClient, err := testHTTPClient(false)
	require.NoError(t, err)

	resp, err := httpClient.Get(fmt.Sprintf("http://127.0.0.1:%d/jwt/v1/help", server.port))
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	help := string(body)
	require.Equal(t, jwtAPIHelp, help)
}

func CreateOperatorKey(t *testing.T) ([]byte, string, nkeys.KeyPair) {
	kp, err := nkeys.CreateOperator()
	require.NoError(t, err)

	seed, err := kp.Seed()
	require.NoError(t, err)

	pub, err := kp.PublicKey()
	require.NoError(t, err)

	return seed, pub, kp
}

func CreateAccountKey(t *testing.T) ([]byte, string, nkeys.KeyPair) {
	kp, err := nkeys.CreateAccount()
	require.NoError(t, err)

	seed, err := kp.Seed()
	require.NoError(t, err)

	pub, err := kp.PublicKey()
	require.NoError(t, err)

	return seed, pub, kp
}

func MakeTempStore(t *testing.T, name string, kp nkeys.KeyPair) (*nsc.Store, string) {
	p, err := ioutil.TempDir("", "store_test")
	require.NoError(t, err)

	var nk *nsc.NamedKey
	if kp != nil {
		nk = &nsc.NamedKey{Name: name, KP: kp}
	}

	s, err := nsc.CreateStore(name, p, nk)
	require.NoError(t, err)
	require.NotNil(t, s)
	return s, p
}

func CreateTestStoreForOperator(t *testing.T, name string, operator nkeys.KeyPair) (*nsc.Store, string) {
	s, p := MakeTempStore(t, name, operator)

	require.NotNil(t, s)
	require.FileExists(t, filepath.Join(s.Dir, ".nsc"))
	require.True(t, s.Has("", ".nsc"))

	if operator != nil {
		tokenName := fmt.Sprintf("%s.jwt", nsc.SafeName(name))
		require.FileExists(t, filepath.Join(s.Dir, tokenName))
		require.True(t, s.Has("", tokenName))
	}

	return s, p
}

func TestStartWithNSCFlag(t *testing.T) {
	_, _, kp := CreateOperatorKey(t)
	_, apub, _ := CreateAccountKey(t)
	s, path := CreateTestStoreForOperator(t, "x", kp)

	c := jwt.NewAccountClaims(apub)
	c.Name = "foo"
	cd, err := c.Encode(kp)
	require.NoError(t, err)
	_, err = s.StoreClaim([]byte(cd))
	require.NoError(t, err)

	flags := Flags{
		DebugAndVerbose: true,
		NSCFolder:       filepath.Join(path, "x"),
		HostPort:        "127.0.0.1:0",
	}

	server := NewAccountServer()
	server.InitializeFromFlags(flags)
	server.config.Logging.Custom = NewNilLogger()
	err = server.Start()
	require.NoError(t, err)
	defer server.Stop()

	httpClient, err := testHTTPClient(false)
	require.NoError(t, err)

	resp, err := httpClient.Get(fmt.Sprintf("http://127.0.0.1:%d/jwt/v1/accounts/%s", server.port, apub))
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	jwt := string(body)
	require.Equal(t, cd, jwt)
}

func TestHostPortFlagOverridesConfigFileFlag(t *testing.T) {
	path, err := ioutil.TempDir(os.TempDir(), "store")
	require.NoError(t, err)

	file, err := ioutil.TempFile(os.TempDir(), "config")
	require.NoError(t, err)

	configString := `
	{
		store: {
			Dir: '%s',
		},
		http: {
			ReadTimeout: 2000,
			Port: 80,
			Host: "nats.io",
		}
	}
	`
	configString = fmt.Sprintf(configString, path)

	fullPath, err := conf.ValidateFilePath(file.Name())
	require.NoError(t, err)

	err = ioutil.WriteFile(fullPath, []byte(configString), 0644)
	require.NoError(t, err)

	flags := Flags{
		ConfigFile: fullPath,
		HostPort:   "127.0.0.1:0",
	}

	server := NewAccountServer()
	err = server.InitializeFromFlags(flags)
	require.NoError(t, err)
	server.config.Logging.Custom = NewNilLogger()
	err = server.Start()
	require.NoError(t, err)
	defer server.Stop()

	require.Equal(t, server.config.Store.Dir, path)
	require.Equal(t, server.config.HTTP.ReadTimeout, 2000)

	httpClient, err := testHTTPClient(false)
	require.NoError(t, err)

	resp, err := httpClient.Get(fmt.Sprintf("http://127.0.0.1:%d/jwt/v1/help", server.port))
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	help := string(body)
	require.Equal(t, jwtAPIHelp, help)
}

func TestStartWithConfigFileFlag(t *testing.T) {
	file, err := ioutil.TempFile(os.TempDir(), "config")
	require.NoError(t, err)

	configString := `
	OperatorJWTPath: "X:/some_path/NATS.jwt"
	systemaccountjwtpath: "X:/some_path/SYS.jwt"
	primary: "http://primary.nats.io:5222"
	http: {
		host: "a.nats.io",
		port: 9090,
		readtimeout: 5000,
		writetimeout: 5000 }
	store: {
		dir: "D:/nats/as_store",
		readonly: false,
		shard: false }
	logging: { 
		debug: true,
		pid: true,
		time: true,
		trace: false,
		colors: true }
	nats: { 
		servers: ["nats://a.nats.io:4243","nats://b.nats.io:4243"], 
		usercredentials: "X:/some_path/admin.creds", 
		ConnectTimeout: 5000,
		ReconnectWait: 10000
	}
	`

	fullPath, err := conf.ValidateFilePath(file.Name())
	require.NoError(t, err)

	err = ioutil.WriteFile(fullPath, []byte(configString), 0644)
	require.NoError(t, err)

	flags := Flags{
		ConfigFile: fullPath,
	}

	server := NewAccountServer()
	err = server.InitializeFromFlags(flags)
	require.NoError(t, err)

	require.Equal(t, "X:/some_path/NATS.jwt", server.config.OperatorJWTPath)
	require.Equal(t, "X:/some_path/SYS.jwt", server.config.SystemAccountJWTPath)
	require.Equal(t, "http://primary.nats.io:5222", server.config.Primary)

	require.Equal(t, "D:/nats/as_store", server.config.Store.Dir)
	require.False(t, server.config.Store.ReadOnly)
	require.False(t, server.config.Store.Shard)

	require.Equal(t, 5000, server.config.HTTP.ReadTimeout)
	require.Equal(t, "a.nats.io", server.config.HTTP.Host)
	require.Equal(t, 9090, server.config.HTTP.Port)

	require.Equal(t, 2, len(server.config.NATS.Servers))
	require.Equal(t, "nats://a.nats.io:4243", server.config.NATS.Servers[0])
	require.Equal(t, "nats://b.nats.io:4243", server.config.NATS.Servers[1])
	require.Equal(t, "X:/some_path/admin.creds", server.config.NATS.UserCredentials)
	require.Equal(t, 5000, server.config.NATS.ConnectTimeout)
	require.Equal(t, 10000, server.config.NATS.ReconnectWait)
}

func TestStartWithBadConfigFileFlag(t *testing.T) {
	server := NewAccountServer()
	err := server.ApplyConfigFile("")
	require.Error(t, err)

	err = server.ApplyConfigFile("/a/b/c")
	require.Error(t, err)

	flags := Flags{
		ConfigFile: "/a/b/c",
	}
	err = server.InitializeFromFlags(flags)
	require.Error(t, err)
}

func TestNATSFlags(t *testing.T) {
	lock := sync.Mutex{}

	// Setup the full environment, but we will make another server to
	// test flags
	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), false, true)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	_, _, kp := CreateOperatorKey(t)
	_, apub, _ := CreateAccountKey(t)
	s, path := CreateTestStoreForOperator(t, "x", kp)

	c := jwt.NewAccountClaims(apub)
	c.Name = "foo"
	cd, err := c.Encode(kp)
	require.NoError(t, err)
	_, err = s.StoreClaim([]byte(cd))
	require.NoError(t, err)

	flags := Flags{
		DebugAndVerbose: true,
		NSCFolder:       filepath.Join(path, "x"),
		HostPort:        "127.0.0.1:0",
		NATSURL:         testEnv.NC.ConnectedUrl(),
		Creds:           testEnv.SystemUserCredsFile,
	}

	server := NewAccountServer()
	err = server.InitializeFromFlags(flags)
	require.NoError(t, err)
	server.config.Logging.Custom = NewNilLogger()
	err = server.Start()
	require.NoError(t, err)
	defer server.Stop()

	httpClient, err := testHTTPClient(false)
	require.NoError(t, err)

	notificationJWT := ""
	subject := fmt.Sprintf(accountNotificationFormat, apub)
	_, err = testEnv.NC.Subscribe(subject, func(m *nats.Msg) {
		lock.Lock()
		notificationJWT = string(m.Data)
		lock.Unlock()
	})
	require.NoError(t, err)

	resp, err := httpClient.Get(fmt.Sprintf("http://127.0.0.1:%d/jwt/v1/accounts/%s?notify=true", server.port, apub))
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	jwt := string(body)
	require.Equal(t, cd, jwt)

	server.nats.Flush()
	testEnv.NC.Flush()

	lock.Lock()
	require.Equal(t, notificationJWT, string(jwt))
	lock.Unlock()
}

func TestStartWithBadHostPortFlag(t *testing.T) {
	_, _, kp := CreateOperatorKey(t)
	_, path := CreateTestStoreForOperator(t, "x", kp)

	flags := Flags{
		DebugAndVerbose: true,
		NSCFolder:       filepath.Join(path, "x"),
		HostPort:        "127.0.0.1",
	}

	server := NewAccountServer()
	err := server.InitializeFromFlags(flags)
	require.Error(t, err)

	flags = Flags{
		DebugAndVerbose: true,
		NSCFolder:       filepath.Join(path, "x"),
		HostPort:        "127.0.0.1:blam",
	}

	err = server.InitializeFromFlags(flags)
	require.Error(t, err)
}

func TestFlagOverridesConfig(t *testing.T) {
	path, err := ioutil.TempDir(os.TempDir(), "store")
	require.NoError(t, err)

	file, err := ioutil.TempFile(os.TempDir(), "config")
	require.NoError(t, err)

	configString := `
	{
		store: {
			Dir: '%s',
			ReadOnly: false,
		},
		http: {
			ReadTimeout: 2000,
			Port: 0,
		}
	}
	`
	configString = fmt.Sprintf(configString, path)

	fullPath, err := conf.ValidateFilePath(file.Name())
	require.NoError(t, err)

	err = ioutil.WriteFile(fullPath, []byte(configString), 0644)
	require.NoError(t, err)

	flags := Flags{
		ConfigFile: fullPath,
		ReadOnly:   true,
		Directory:  path,
	}

	server := NewAccountServer()
	err = server.InitializeFromFlags(flags)
	require.NoError(t, err)
	server.config.Logging.Custom = NewNilLogger()
	err = server.Start()
	require.NoError(t, err)
	defer server.Stop()

	require.Equal(t, server.config.Store.Dir, path)
	require.Equal(t, server.config.HTTP.ReadTimeout, 2000)
	require.True(t, server.jwtStore.IsReadOnly())
}
