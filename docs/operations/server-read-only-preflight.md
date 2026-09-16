# Server Read-Only Preflight

**Date:** 2026-09-06  
**Target:** `192.168.78.138`  
**Requested deployment root:** `/opt/aetheris`

## Local checks

- Working directory: `D:\New_Project\Aetheris`
- Local user: `desktop-1t1hrvf\\lujinming`
- SSH directory contains an RSA keypair and known-hosts files.
- No server password was read, requested, or written to this repository.

## Remote attempt

The following read-only connection probe was attempted with batch mode and a five-second timeout:

```text
ssh -o BatchMode=yes -o ConnectTimeout=5 -o StrictHostKeyChecking=accept-new 192.168.78.138 "id -u; id -un"
```

Result: exit code `1`; the host key was recorded, then the server returned `Permission denied (publickey,password)` for the local default user. No password authentication was attempted.

## Safety result

- No command was run on `/opt`.
- No file, directory, permission, owner, service, or disk state was changed.
- Deployment remains blocked until a non-root service account and an authorized SSH public key are provided through the approved administration channel.

## Deployment exception and result

The owner explicitly authorized one-time root password authentication for deployment. The password was entered interactively over SSH and was not placed in any command argument, file, repository content, or log.

Deployment completed to `/opt/aetheris` with:

- non-root service account `aetheris`;
- systemd unit `aetheris-gateway.service`, enabled at boot;
- Gateway listening on `0.0.0.0:8080`;
- token stored in `/etc/aetheris/gateway.env` with `root:aetheris 0640` permissions;
- application data in `/var/lib/aetheris` owned by `aetheris`;
- `GET /healthz` returning HTTP 200 with `{"status":"ok"}`;
- process command line referencing only the token file, not the token value.

Static Windows Core delivery was then enabled:

- install page: `http://192.168.78.138:8080/`;
- catalog: `http://192.168.78.138:8080/downloads/`;
- bundle: `http://192.168.78.138:8080/downloads/aetheris-core-beta.zip`;
- checksum: `http://192.168.78.138:8080/downloads/aetheris-core-beta.sha256`;
- remote checksum verification passed before service restart;
- local HTTP probes returned 200 for the page, catalog, and zip (`application/zip`, 14,286 bytes).

The delivery was simplified to a versioned direct tray executable. The public catalog now exposes `AetherisCore-0.2.0.exe` and its checksum; prior IExpress setup and unversioned executable URLs were removed from the Gateway allowlist. The executable checksum passed on the server, and the service remained `active/running` after restart.

The Gateway now also exposes the authenticated device-management view at `/admin`, backed by `/v1/devices`, `/v1/summary`, `/v1/events`, and `/v1/export/events.<format>`. Personal review is local-only via the Core tray's loopback workspace. These endpoints were covered by local tests before deployment.
