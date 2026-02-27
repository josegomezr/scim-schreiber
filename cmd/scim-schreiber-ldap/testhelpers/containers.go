package testhelpers

import (
	"context"
	"fmt"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type LdapContainer struct {
	*testcontainers.DockerContainer
	BaseDN string
}

func CreateLdapContainer(ctx context.Context) (*LdapContainer, error) {
	ldapContainer, err := testcontainers.Run(
		ctx, "registry.suse.com/suse/389-ds:2.5",
		/*testcontainers.WithDockerfile(testcontainers.FromDockerfile{
			Context:    filepath.Join(".", "testdata"),
			Dockerfile: "389.Dockerfile",
			KeepImage:  true,
		}),*/
		testcontainers.WithExposedPorts("3389/tcp", "3636/tcp"),
		testcontainers.WithEnv(map[string]string{
			"DS_DM_PASSWORD": "changeme",
		}),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("3389/tcp"),
			wait.ForLog("INFO: 389-ds-container started"),
		),
	)
	if err != nil {
		return nil, err
	}

	baseDN := "dc=test,dc=org"
	exitCode, _, err := ldapContainer.Exec(ctx, []string{"dsconf", "localhost", "backend", "create", "--suffix", baseDN, "--be-name", "userroot", "--create-suffix", "--create-entries"})
	if err != nil {
		return nil, err
	}

	if exitCode != 0 {
		return nil, fmt.Errorf("failed to create backend: exit %d", exitCode)
	}

	// Now we can create a user
	// dsidm localhost --basedn dc=suse,dc=com user create --uid ldap_user --cn ldap_user --displayName ldap_user --uidNumber 1001 --gidNumber 1001 --homeDirectory /home/ldap_user

	return &LdapContainer{
		DockerContainer: ldapContainer,
		BaseDN:          baseDN,
	}, nil
}

func (c *LdapContainer) GetEndpoint(ctx context.Context) (string, error) {
	return c.PortEndpoint(ctx, "3389/tcp", "ldap")
}
