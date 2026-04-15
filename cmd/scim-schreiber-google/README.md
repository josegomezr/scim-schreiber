# Prerequisite
## Setup Google Service account

### Create the service account
- Go to [Service Accounts](https://console.cloud.google.com/iam-admin/serviceaccounts)
- If you have no project there create one.
- Click on `Create service account`
- Enter the name and the account id. Click on `Create and continue`
- Note the generated Client ID
- Switch the `Keys` tab of the service account you just created.
- Click on `Add key` > `Create new key` > `JSON`
- Download the key and store it in a safe place (e.g. `/etc/scim-schreiber-google/service-account.json`).


### Configure service account permissions
- Go to the [Admin UI](https://admin.google.com/) 
- Navigate to `Security > Access and data control > API controls`
- Click on `MANAGE DOMAIN WIDE DELEGATION`
- Click on `Add new`
- Enter the client ID from the service account creation
- Add the following OAuth scopes:
  - https://www.googleapis.com/auth/admin.directory.user
  - https://www.googleapis.com/auth/admin.directory.group
  - https://www.googleapis.com/auth/apps.licensing
  - https://www.googleapis.com/auth/admin.directory.orgunit
- Click `Authorize`

### Create imitation user
- Go to the [Admin UI](https://admin.google.com/)
- Navigate to `Directory > Users`
- Create a new user by clicking on `Add new user`
- Enter a `First name` and `Last name` by which you can identify the service account
- Finish creating the user
- Navigate to the user you just created.
- Under `User Details` scroll to `Admin roles and privileges.
- Assign the `Super Admin` permission or create a custom role with only the required permissions.

# Run configuration
## Environment Variables

| Name               | Description                                              | Example                                         |
|--------------------|----------------------------------------------------------|-------------------------------------------------|
| GOOGLE_CREDENTIALS | The path to the key file created for the service account. | /etc/scim-schreiber-google/service-account.json |
| GOOGLE_DOMAIN      | The domain that should be managed on Google Workspace    | example.com                                     |
| GOOGLE_SUBJECT     | The subject to imitate with the service account.         | admin@example.com                               |


# References
[Service Account Docs](https://developers.google.com/identity/protocols/oauth2/service-account#creatinganaccount)