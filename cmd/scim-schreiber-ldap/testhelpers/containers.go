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

	baseDN := "dc=suse,dc=com"
	exitCode, _, err := ldapContainer.Exec(ctx, []string{"dsconf", "localhost", "backend", "create", "--suffix", baseDN, "--be-name", "userroot", "--create-suffix", "--create-entries"})
	if err != nil {
		return nil, err
	}

	if exitCode != 0 {
		return nil, fmt.Errorf("failed to create backend: exit %d", exitCode)
	}

	exitCode, _, err = ldapContainer.Exec(ctx, []string{"bash", "-c", `
	set -e
    dsconf localhost schema attributetypes add isActive --oid=1.3.6.1.4.1.7057.340.1.1.2 --desc="Account is active" --syntax=1.3.6.1.4.1.1466.115.121.1.7 --single-value --x-origin="user defined" --equality=booleanMatch
	dsconf localhost schema attributetypes add sshPublicKey --oid=1.3.6.1.4.1.7057.340.1.1.12 --desc="SSH public keys" --syntax=1.3.6.1.4.1.1466.115.121.1.15 --single-value --x-origin="user defined" --equality=caseIgnoreMatch
	dsconf localhost schema attributetypes add gpgPublicKey --oid=1.3.6.1.4.1.7057.340.1.1.13 --desc="GPG public keys" --syntax=1.3.6.1.4.1.1466.115.121.1.15 --single-value --x-origin="user defined" --equality=caseIgnoreMatch
	dsconf localhost schema attributetypes add communityUid --oid=1.3.6.1.4.1.7057.340.1.1.14 --desc="Community unique user name" --syntax=1.3.6.1.4.1.1466.115.121.1.15 --single-value --x-origin="user defined" --equality=caseIgnoreMatch
	dsconf localhost schema attributetypes add entitlements --oid=1.3.6.1.4.1.7057.340.1.1.18 --desc="Flags used in other services" --syntax=1.3.6.1.4.1.1466.115.121.1.15 --multi-value --x-origin="user defined" --equality=caseIgnoreMatch
	dsconf localhost schema attributetypes add uuid --oid=1.3.6.1.4.1.7057.340.1.1.19 --desc="Unique unit identifier" --syntax=1.3.6.1.4.1.1466.115.121.1.15 --single-value --x-origin="user defined" --equality=caseIgnoreMatch
	dsconf localhost schema objectclasses add suseuser --oid=1.3.6.1.4.1.7057.340.1.1 --desc="SUSE user object" --sup=inetOrgPerson --kind=STRUCTURAL --must uid employeeNumber isActive --may sshPublicKey gpgPublicKey communityUid entitlements uuid
    `})
	if err != nil {
		return nil, err
	}

	if exitCode != 0 {
		return nil, fmt.Errorf("failed to create backend: exit %d", exitCode)
	}

	return &LdapContainer{
		DockerContainer: ldapContainer,
		BaseDN:          baseDN,
	}, nil
}

func (c *LdapContainer) GetEndpoint(ctx context.Context) (string, error) {
	return c.PortEndpoint(ctx, "3636/tcp", "ldaps")
}
