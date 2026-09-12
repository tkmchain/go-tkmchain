#!/usr/bin/env python3
"""Generate Windows version resources; publisher metadata is not a digital signature."""
import argparse
from pathlib import Path
import re
import subprocess
import tempfile
p = argparse.ArgumentParser()
p.add_argument('--publisher', required=True)
p.add_argument('--version', default='0.5.0')
p.add_argument('--windres', default='x86_64-w64-mingw32-windres')
a = p.parse_args()
if not a.publisher.strip() or any(ord(c) < 32 for c in a.publisher):
    p.error('publisher must be a nonempty printable name')
if not re.fullmatch(r'\d{1,5}\.\d{1,5}\.\d{1,5}(?:\.\d{1,5})?', a.version):
    p.error('version must contain three or four numeric components')
parts = [int(v) for v in a.version.split('.')]
if any(v > 65535 for v in parts): p.error('version components must be at most 65535')
parts += [0] * (4-len(parts))
def quote(s): return s.replace('\\', '\\\\').replace('"', '\\"')
root = Path(__file__).resolve().parents[2]
for directory, filename, description in [('gtkm','TKM-Wallet-Windows-x64.exe','TKM Wallet'),('shielded-payout-prover','shielded-payout-prover.exe','TKM Wallet Local Proof Builder')]:
    fields = {'CompanyName':a.publisher.strip(), 'FileDescription':description, 'FileVersion':a.version, 'InternalName':directory, 'OriginalFilename':filename, 'ProductName':'TKM Wallet', 'ProductVersion':a.version}
    values = '\n'.join(f'VALUE "{key}", "{quote(value)}\\0"' for key,value in fields.items())
    resource = f'''#include <windows.h>
1 VERSIONINFO
FILEVERSION {','.join(map(str,parts))}
PRODUCTVERSION {','.join(map(str,parts))}
FILEFLAGSMASK 0x3fL
FILEFLAGS 0x0L
FILEOS VOS_NT_WINDOWS32
FILETYPE VFT_APP
FILESUBTYPE 0x0L
BEGIN
 BLOCK "StringFileInfo"
 BEGIN
  BLOCK "040904b0"
  BEGIN
{values}
  END
 END
 BLOCK "VarFileInfo"
 BEGIN
  VALUE "Translation", 0x409, 1200
 END
END
'''
    with tempfile.TemporaryDirectory(prefix='tkm-version-') as tmp:
        rc=Path(tmp)/'version.rc'; rc.write_text(resource,encoding='utf-8')
        subprocess.run([a.windres,'--codepage=65001','-i',str(rc),'-o',str(root/'cmd'/directory/'wallet_version_windows_amd64.syso')],check=True)
print('Windows publisher and version resources generated for wallet and prover.')
