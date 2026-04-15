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
- Download the key and store it in a safe place.


### Configure service account permissions
- Go to the [Admin UI](https://admin.google.com/) 
- Navigate to `Security > Access adn data control > API controls`
- Click on `MANAGE DOMAIN WIDE DELEGATION`
- Click on `Add new`
- Enter the client ID from the service account creation
- Add the following OAuth scopes:
  - https://www.googleapis.com/auth/admin.directory.user
  - https://www.googleapis.com/auth/admin.directory.group
  - https://www.googleapis.com/auth/apps.licensing
  - https://www.googleapis.com/auth/admin.directory.orgunit
- Click `Authorize`


# References
[Service Account Docs](https://developers.google.com/identity/protocols/oauth2/service-account#creatinganaccount)