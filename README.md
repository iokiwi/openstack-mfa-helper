# Catalyst Cloud MFA Helper

Proof of concept/Work in progress

Client configuration method for authenticating with Catalyst Cloud - inspired by https://github.com/broamski/aws-mfa

## The problem

Openstack provides several methods for configuring authentication for its SDK's and clients including CLI arguments, environment variables and a static clouds.yaml file.

For some reason the perferred / default authentication menthod method Catalyst Cloud encourage people to use is using the openrc files - non-trivial bash files which set environment variables in your shell session. I know as I wrote the damn thing.

Using environment varaibles for configuration has several drawbacks mainly that Environment variables 
are not very durable or must be set in every new shell session

The static [clouds.yaml configuration file](https://docs.openstack.org/python-openstackclient/latest/configuration/index.html#configuration-files) only needs to be created once and provides persistent authentication across any number of terminal sessions or machine restarts. While clouds.yaml is clearly more convenient, it has a couple of downsides as well.

* It encourages storing usernames and passwords in clear text
* It doesn't work that well with MFA

This helper script addresses these problems by automating the management of your clouds.yml file.

## Install

```bash
$ mkdir -p $HOME/.local/bin
$ curl https://raw.githubusercontent.com/iokiwi/openstack-mfa-helper/main/cc-mfa.py -o $HOME/.local/bin/cc-mfa.py
$ chmod +x $HOME/.local/bin/cc-mfa.py
$ export PATH=$HOME/.local/bin:$PATH
# ^^^ Add this line to ~/.bashrc or ~/.profile etc.
```

## Usage

1. Obtain a `clouds.yaml` file from the cloud dashboard
    * Go to https://dashboard.cloud.catalyst.net.nz/project/api_access/
    *  Hover over `Download OpenStack RC File` on the top right
    * Click the `OpenStack clouds.yml file`
    * If you are logged in already you may just be able to click this link to [Download clouds.yml](https://dashboard.cloud.catalyst.net.nz/project/api_access/clouds.yaml/)

2. Place your `clouds.yaml` file in one of the following locations detailed in the [upstream docs](https://docs.openstack.org/python-openstackclient/latest/cli/man/openstack.html#config-files)
    * `./clouds.yaml`
    * `~/config/openstack/clouds.yaml`
    * `/etc/openstack/clouds.yaml`

3. Run cc-mfa.py

    ```bash
    $ export OS_CLOUD=catalystcloud
    $ cc-mfa.py
    ```

    Alternatively you can do it all in one line but OS_CLOUD will not be exported and will need to be set for future invocations of the openstack clients.

    ``` 
    $ OS_CLOUD=catalystcloud cc-mfa.py
    ```

4. It will create `static-clouds.yaml` file in the same directory as your `clouds.yaml` configuration
5. And configure `clouds.yaml` with token based authorisation

Before

```yml
# clouds.yaml
clouds:
  catalystcloud:
    auth:
      auth_url: https://api.nz-hlz-1.catalystcloud.io:5000
      username: "john.smith@example.com"
      project_id: 1a2b3c4d5e6f7g8h9i0j1l2m3n4o5p6
      project_name: "johns-project"
      user_domain_name: "Default"
    region_name: "nz-hlz-1"
    interface: "public"
    identity_api_version: 3
```

After

```yml
# clouds.yaml
clouds:
  catalystcloud:
    auth_type: token
    auth:
      auth_url: https://api.nz-hlz-1.catalystcloud.io:5000
      project_id: 1a2b3c4d5e6f7g8h9i0j1l2m3n4o5p6
      project_name: "johns-project"
      token: "gAAAAABiDIpLavmw9Bsj2oOL..."
    region_name: "nz-hlz-1"
    interface: "public"
    identity_api_version: 3
```

```yml
# static-clouds.yml
clouds:
  catalystcloud:
    auth:
      auth_url: https://api.nz-hlz-1.catalystcloud.io:5000
      username: "john.smith@example.com"
      project_id: 1a2b3c4d5e6f7g8h9i0j1l2m3n4o5p6
      project_name: "johns-project"
      user_domain_name: "Default"
    region_name: "nz-hlz-1"
    interface: "public"
    identity_api_version: 3
```

6. Then use `clouds.yaml` for authentication as per normal. More details: https://docs.openstack.org/python-openstackclient/latest/configuration/index.html#configuration-files

```bash
$ export OS_CLOUD=catalystcloud
$ openstack server list
+---------------+---------------+--------+---------------+---------------+---------+
| ID            | Name          | Status | Networks      | Image         | Flavor  |
+---------------+---------------+--------+---------------+---------------+---------+
| a5d3814a-4f6e | proxy-server- | ACTIVE | proxy-        | N/A (booted   | c1.c1r1 |
| -493f-aa03-ac | yohjuqh5vzhm  |        | network-lju2p | from volume)  |         |
| 13fdb9a860    |               |        | kokjtsh=10.0. |               |         |
|               |               |        | 0.4, 103.***. |               |         |
|               |               |        | ***.***       |               |         |
+---------------+---------------+--------+---------------+---------------+---------+
```

7. If your token expires run `cc-mfa.py` again to get `clouds.yaml` updated with a fresh token.