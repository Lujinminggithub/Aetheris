# Aetheris Core Service ProcMon Diagnosis

**Date:** 2026-09-08  
**Tool:** Microsoft Sysinternals Process Monitor 4.04, Microsoft signature verified  
**Result:** Third-party endpoint protection deletes the service executable during installation/startup.

## Evidence

- `21:51:49.7766568`: Setup creates `AetherisCoreService.exe.new` successfully.
- `21:51:49.7782239`: Setup rename is denied while the file is being scanned.
- From `21:51:49.779`: `360Tray.exe` repeatedly opens and reads the staged and final service images.
- From `21:51:49.972`: `360Tray.exe` repeatedly changes the final file DACL and owner.
- `21:51:50.1164408`: `services.exe` successfully creates the service process, PID 828.
- `21:51:50.1180240`: the service process exits before entering normal service initialization.
- `21:51:50.1478196`: `360Tray.exe` successfully marks `AetherisCoreService.exe` for deletion using `SetDispositionInformationEx`.

The service image path, file mapping, ACL check, Authenticode `/pa` verification, and process creation all succeed before the endpoint product removes the image. The installer and service must not attempt to weaken or bypass endpoint protection. Local testing requires an administrator-approved 360 trust rule for the signed service publisher/hash and exact install directory.
