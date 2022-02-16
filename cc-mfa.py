#!/usr/bin/env python3

import yaml
import os
from pathlib import Path
import sys

import requests
import json
from getpass import getpass


def find_config_path(name: str) -> Path:

    search = [
        ".",
        "~/.config/openstack",
        "/etc/openstack",
    ]

    for location in search:
        if os.path.exists(location + "/" + name):
            return Path(location)
    raise FileNotFoundError


def load_config(path: Path) -> dict:
    with open(path, "r") as f:
        config = yaml.safe_load(f)
    return config


def get_token(username, password, user_domain_name):

    r = requests.post(
        auth_url + "/v3/auth/tokens",
        headers={"Content-Type": "application/json"},
        data=json.dumps(
            {
                "auth": {
                    "identity": {
                        "methods": ["password"],
                        "password": {
                            "user": {
                                "name": username,
                                "domain": {"name": user_domain_name},
                                "password": password,
                            }
                        },
                    },
                    "scope": {"project": {"id": project_id}},
                }
            }
        ),
    )

    return r.headers.get("x-subject-token")


if __name__ == "__main__":

    os_cloud = os.environ.get("OS_CLOUD")
    if os_cloud is None:
        sys.exit("$OS_CLOUD must be set in your environment")

    # Part 1: Look for clouds.yaml or static-clouds.yaml
    try:
        config_path = find_config_path("static-clouds.yaml")
    except FileNotFoundError:

        print("static-clouds.yaml not found")

        config_path = find_config_path("clouds.yaml")

        # Dump clouds.yaml into static-clouds.yaml
        with open(config_path / "clouds.yaml", "r") as f1:
            content = yaml.safe_load(f1)

            if "auth_type" in content["clouds"][os_cloud]:
                del content["clouds"][os_cloud]["auth_type"]

            if "token" in content["clouds"][os_cloud]["auth"]:
                del content["clouds"][os_cloud]["auth"]["token"]

            with open(config_path / "static-clouds.yaml", "w") as f2:
                print("Creating", config_path / "static-clouds.yaml")
                yaml.dump(content, f2)

    # Part 2: Get token
    config = load_config(config_path / "static-clouds.yaml")

    cloud_config = config["clouds"][os_cloud]

    user_domain_name = cloud_config["auth"].get("user_domain_name", "Default")
    project_id = cloud_config["auth"]["project_id"]
    auth_url = cloud_config["auth"]["auth_url"]

    username = cloud_config["auth"].get("username")
    if username is None:
        username = input("Username:").strip()
        # TODO: Offer to save username in static-config.yml for next time

    password = cloud_config["auth"].get("password")
    if password is None:
        password = getpass(f"Password for {username}:")
    else:
        # TODO: Suggest/offer to delete password from static-config.yml
        pass

    totp = input("MFA Temporary Password (Press enter to skip):").strip()
    password = password + totp

    print("Getting token...")
    token = get_token(username, password, user_domain_name)

    # Part 3: Update clouds.yaml file
    if not os.path.exists(config_path / "clouds.yaml"):
        ephemeral_config = load_config(config_path / "static-clouds.yaml")
        with open(config_path / "clouds.yaml", "w") as f:
            pass
    else:
        ephemeral_config = load_config(config_path / "clouds.yaml")

    # set auth_type = token
    ephemeral_config["clouds"][os_cloud]["auth_type"] = "token"

    # set token
    auth = ephemeral_config["clouds"][os_cloud]["auth"]
    auth["token"] = token

    # Clear unwanted auth attributes
    for x in ["username", "password", "user_domain_name"]:
        if x in auth:
            del auth[x]
    ephemeral_config["clouds"][os_cloud]["auth"] = auth

    # Write modified config to clouds.yaml
    print("Updating", config_path / "clouds.yaml")
    with open(config_path / "clouds.yaml", "w") as f:
        yaml.dump(ephemeral_config, f)
