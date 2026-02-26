package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	reusableContainerName = "my_test_reusable_container"
)

func TestTwo(t *testing.T) {
	logger := log.Default()
	ctx := context.Background()
	logger.Println("Starting testcontainers 1.")
	runContainer(ctx, t)
	logger.Println("Starting testcontainers 2.")
	runContainer(ctx, t)
}

func runContainer(ctx context.Context, t *testing.T) {

	ldapC, err := testcontainers.Run(
		ctx, "registry.suse.com/suse/389-ds:2.5",
		testcontainers.WithExposedPorts("3389/tcp", "3636/tcp"),
		testcontainers.WithEnv(map[string]string{
			"DS_DM_PASSWORD": "changeme",
		}),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("3389/tcp"),
			wait.ForLog("INFO: 389-ds-container started"),
		),
		testcontainers.WithLogger(log.Default()),
		testcontainers.WithReuseByName(reusableContainerName),
	)

	require.NoError(t, err)

	defer testcontainers.CleanupContainer(t, ldapC)

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
