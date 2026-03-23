import json
import logging
import os
from wsgiref.simple_server import make_server
from wsgiref.util import is_hop_by_hop

import requests

PORT = int(os.environ.get("PORT", "8000"))
LOG_LEVEL = os.environ.get("LOG_LEVEL", "INFO")

logging.basicConfig()
logger = logging.getLogger("sfdc-proxy")
logger.setLevel(LOG_LEVEL)

HEADERS_FOR_PROXY_IF_POST_PATCH = {
    "CONTENT_LENGTH": "Content-length",
    "CONTENT_TYPE": "Content-type",
}
HEADERS_FOR_PROXY = {
    "HTTP_ACCEPT": "Accept",
    "HTTP_AUTHORIZATION": "Authorization",
    "HTTP_USER_AGENT": "User-Agent",
}

KNOWN_HEADERS = {
    **HEADERS_FOR_PROXY,
    **HEADERS_FOR_PROXY_IF_POST_PATCH,
    "HTTP_HOST": "Host",
}

DEFAULT_PERMS = {
    "UserPermissionsMarketingUser": False,
    "UserPermissionsOfflineUser": False,
    "UserPermissionsAvantgoUser": False,
    "UserPermissionsCallCenterAutoLogin": False,
    "UserPermissionsSFContentUser": False,
    "UserPermissionsKnowledgeUser": False,
    "UserPermissionsInteractionUser": False,
    "UserPermissionsSupportUser": False,
}


def get_headers(environ):
    return {
        KNOWN_HEADERS.get(k, k): v
        for k, v in environ.items()
        if k.startswith("HTTP_") or k in KNOWN_HEADERS
    }


def headers_for_proxy(environ, method):
    return {
        KNOWN_HEADERS[k]: v
        for k, v in environ.items()
        if k in HEADERS_FOR_PROXY
        or (k in HEADERS_FOR_PROXY_IF_POST_PATCH and (method in ["POST", "PATCH"]))
    }


def get_body(environ):
    try:
        request_body_size = int(environ.get("CONTENT_LENGTH", 0))
    except ValueError:
        request_body_size = 0
    if "wsgi.input" in environ:
        return environ["wsgi.input"].read(request_body_size)
    return


class DaApp:
    server_software = "sfdc-proxy/1.0.0"

    def __init__(self, sfdc_base_url):
        self.sfdc_base_url = sfdc_base_url

    def _get_user_perms_from_create_request(self, payload):
        return payload.get("_to_patch_request", {})

    def _get_user_uid_and_perms_from_patch_request(self, payload):
        return payload["Operations"][0]["value"].get("_to_patch_request", {})

    def update_user_perms(self, uid, perms, bearer):
        url = f"{self.sfdc_base_url}/services/data/v60.0/sobjects/User/{uid}"
        logger.debug(
            f"Post-Patch request to the SFDC Data API {dict(url=url, data=perms)}"
        )
        return requests.request(
            "PATCH", url, json=perms, headers=dict(Authorization=bearer)
        )

    def __call__(self, environ, start_response):
        path = environ.get("PATH_INFO")
        method = environ.get("REQUEST_METHOD")

        if path == "/-/health" and method == "GET":
            start_response("200 OK", [])
            return b""

        logger.info(f"Incoming request: {dict(path=path, method=method)}")
        logger.debug(
            f"Incoming request: {dict(path=path, method=method, headers=get_headers(environ))}"
        )
        payload = get_body(environ)

        prox_headers = headers_for_proxy(environ, method=method)

        sfdc_scim_url = self.sfdc_base_url + path
        logger.debug(
            "Proxy request: {}".format(
                dict(url=sfdc_scim_url, payload=payload, headers=prox_headers)
            )
        )
        resp = requests.request(
            method,
            sfdc_scim_url,
            headers=prox_headers,
            data=payload,
        )
        logger.debug(
            f"Proxy response: {dict(url=resp.status_code, response=resp.headers, headers=resp.content)}"  # noqa:E501
        )
        resp_body = None
        try:
            resp_body = resp.json()
        except requests.exceptions.JSONDecodeError:
            pass

        uid = None
        userPerms = None
        try:
            resp.raise_for_status()
            # if we're getting a post/put over SCIM, then do the post-req patch
            if method in ["POST", "PATCH"]:
                parsed_json = json.loads(payload)
                if method == "POST":
                    uid = resp_body["id"]
                    userPerms = self._get_user_perms_from_create_request(parsed_json)
                elif method == "PATCH":
                    uid = path.split("/")[-1]
                    userPerms = self._get_user_uid_and_perms_from_patch_request(
                        parsed_json
                    )
        except requests.HTTPError:
            pass

        if uid:
            perms_payload = dict(**DEFAULT_PERMS)
            perms_payload.update(userPerms)

            post_perm_resp = self.update_user_perms(
                uid, perms_payload, prox_headers.get("Authorization")
            )

            new_resp_json = None
            try:
                new_resp_json = post_perm_resp.json()
            except requests.exceptions.JSONDecodeError:
                pass

            resp_body["post-patch-response"] = [
                post_perm_resp.status_code,
                new_resp_json,
            ]

        response = [
            f"{resp.status_code} {resp.reason}",
            [
                (k, v)
                for k, v in resp.headers.items()
                if not is_hop_by_hop(k) and k not in ["Content-Encoding"]
            ],
            [json.dumps(resp_body).encode("utf-8")],
        ]

        status, response_headers, body = response
        logger.info(
            f"Issuing response: {dict(status=status, response_headers=response_headers)}"
        )
        start_response(status, response_headers)
        return body


app = DaApp(os.environ["SFDC_BASE_URL"])

if __name__ == '__main__':
    httpd = make_server("", PORT, app)
    logger.info(f"Listening to *:{PORT}")
    logger.info("Will forward requests to: {}".format(os.environ["SFDC_BASE_URL"]))
    httpd.serve_forever()
