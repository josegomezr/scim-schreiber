package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	reusableContainerName = "my_test_reusable_container"
)

type certs struct {
	ca         *bytes.Buffer
	serverCert *bytes.Buffer
	serverKey  *bytes.Buffer
}

func createCerts() (*certs, error) {
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(2019),
		Subject: pkix.Name{
			CommonName:   reusableContainerName,
			Organization: []string{"testing"},
			Country:      []string{"AU"},
			Province:     []string{"Queensland"},
			Locality:     []string{"389ds"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)

	if err != nil {
		return nil, err
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return nil, err
	}

	caPEM := new(bytes.Buffer)
	err = pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})
	if err != nil {
		return nil, err
	}

	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1658),
		Subject: pkix.Name{
			Organization:  []string{"Company, INC."},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"San Francisco"},
			StreetAddress: []string{"Golden Gate Bridge"},
			PostalCode:    []string{"94016"},
		},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	certPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, err
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, ca, &certPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return nil, err
	}

	certPEM := new(bytes.Buffer)
	err = pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})
	if err != nil {
		return nil, err
	}

	certPrivKeyPEM := new(bytes.Buffer)
	err = pem.Encode(certPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
	})
	if err != nil {
		return nil, err
	}

	return &certs{
		ca:         caPEM,
		serverCert: certPEM,
		serverKey:  certPrivKeyPEM,
	}, nil
}

func TestB(t *testing.T) {
	logger := log.Default()

	ctx := context.Background()

	logger.Println("Starting testcontainers 1.")
	runContainer(ctx, t)
}

func TestA(t *testing.T) {
	logger := log.Default()

	ctx := context.Background()

	logger.Println("Starting testcontainers 2.")
	runContainer(ctx, t)
}

// StdoutLogConsumer is a LogConsumer that prints the log to stdout
type StdoutLogConsumer struct{}

// Accept prints the log to stdout
func (lc *StdoutLogConsumer) Accept(l testcontainers.Log) {
	fmt.Print(string(l.Content))
}

func runContainer(ctx context.Context, t *testing.T) {

	logConsumer := StdoutLogConsumer{}

	ldapC, err := testcontainers.Run(
		ctx, "",
		testcontainers.WithDockerfile(testcontainers.FromDockerfile{
			Context:    filepath.Join(".", "testdata"),
			Dockerfile: "389.Dockerfile",
			KeepImage:  true,
		}),
		testcontainers.WithExposedPorts("3389/tcp", "3636/tcp"),
		testcontainers.WithEnv(map[string]string{
			"DS_DM_PASSWORD": "changeme",
		}),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("3389/tcp"),
			wait.ForLog("INFO: 389-ds-container started"),
		),
		testcontainers.WithLogger(log.Default()),
		/*testcontainers.WithFiles(
			testcontainers.ContainerFile{
				Reader:            certs.ca,
				ContainerFilePath: "/data/tls/ca/ca.crt",
			},
			testcontainers.ContainerFile{
				Reader:            certs.serverCert,
				ContainerFilePath: "/data/tls/server.crt",
			},
			testcontainers.ContainerFile{
				Reader:            certs.serverKey,
				ContainerFilePath: "/data/tls/server.key",
			},
		),*/
		testcontainers.WithLogConsumerConfig(&testcontainers.LogConsumerConfig{
			Opts:      []testcontainers.LogProductionOption{testcontainers.WithLogProductionTimeout(10 * time.Second)},
			Consumers: []testcontainers.LogConsumer{&logConsumer},
		}),
	)

	require.NoError(t, err)

	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(ldapC); err != nil {
			t.Fatalf("failed to terminate pgContainer: %s", err)
		}
	})

	endpoint, err := ldapC.PortEndpoint(ctx, "3389/tcp", "ldap")
	require.NoError(t, err)

	basedn := fmt.Sprintf("dc=test-pid-%d-%d,dc=test,dc=org", os.Getpid(), time.Now().UnixMilli())
	exit_code, _, err := ldapC.Exec(ctx, []string{"dsconf", "localhost", "backend", "create", "--suffix", basedn, "--be-name", "userroot", "--create-suffix", "--create-entries"})
	require.NoError(t, err)
	require.Equal(t, 0, exit_code)

	// Now we can create a user
	// dsidm localhost --basedn dc=suse,dc=com user create --uid ldap_user --cn ldap_user --displayName ldap_user --uidNumber 1001 --gidNumber 1001 --homeDirectory /home/ldap_user

	ldap := LdapUtil{
		ldapEndpoint: endpoint,
		ldapBindDn:   "cn=Directory Manager",
		ldapBindPw:   "changeme",
		baseUserOu:   "ou=people",
		baseGroupOu:  "ou=groups",
		baseDn:       basedn,
	}

	err = ldap.connect()
	require.NoError(t, err)

	entry := ldap.searchUser("demo_user")
	require.NotNil(t, entry)

}
