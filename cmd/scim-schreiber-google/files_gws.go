//go:build google_custom

package main

import _ "embed"

//go:embed testdata/expectations/users_gws/filter-users.json
var filterUserExpectation string

//go:embed testdata/expectations/users_gws/all-users.json
var allUsersExpectation string

//go:embed testdata/expectations/users_gws/get-user.json
var getUserExpectation string

//go:embed testdata/expectations/users_gws/patch-user.json
var patchUserExpectation string

//go:embed testdata/expectations/users_gws/replace-user.json
var replaceUserExpectation string

//go:embed testdata/expectations/users_gws/replace-user-request.json
var replaceUserRequestExpectation string
