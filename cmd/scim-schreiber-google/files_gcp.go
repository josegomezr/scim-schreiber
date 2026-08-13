//go:build !google_custom

package main

import _ "embed"

//go:embed testdata/expectations/users_gcp/filter-users.json
var filterUserExpectation string

//go:embed testdata/expectations/users_gcp/all-users.json
var allUsersExpectation string

//go:embed testdata/expectations/users_gcp/get-user.json
var getUserExpectation string

//go:embed testdata/expectations/users_gcp/patch-user.json
var patchUserExpectation string

//go:embed testdata/expectations/users_gcp/replace-user.json
var replaceUserExpectation string

//go:embed testdata/expectations/users_gcp/replace-user-request.json
var replaceUserRequestExpectation string
