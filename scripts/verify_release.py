"""Validate the Plugin Store's complete six-platform release contract."""
import argparse
import hashlib
import re
import stat
import struct
import sys
import zipfile
from pathlib import Path

PLUGIN_ID = 'team-routing'
PLATFORMS = {
    ('windows', 'amd64'): 'dll',
    ('windows', 'arm64'): 'dll',
    ('linux', 'amd64'): 'so',
    ('linux', 'arm64'): 'so',
    ('darwin', 'amd64'): 'dylib',
    ('darwin', 'arm64'): 'dylib',
}


def verify_architecture(data, goos, goarch):
    try:
        if goos == 'windows':
            offset = struct.unpack_from('<I', data, 0x3c)[0]
            machine = 0x8664 if goarch == 'amd64' else 0xaa64
            valid = data[:2] == b'MZ' and data[offset:offset + 4] == b'PE\0\0' and struct.unpack_from('<H', data, offset + 4)[0] == machine
        elif goos == 'linux':
            machine = 62 if goarch == 'amd64' else 183
            valid = data[:6] == b'\x7fELF\x02\x01' and struct.unpack_from('<H', data, 18)[0] == machine
        else:
            machine = 0x1000007 if goarch == 'amd64' else 0x100000c
            valid = data[:4] == b'\xcf\xfa\xed\xfe' and struct.unpack_from('<I', data, 4)[0] == machine
    except struct.error:
        valid = False
    if not valid:
        raise ValueError(f'Incorrect binary architecture for {goos}/{goarch}')


def verify_release(directory, version):
    if not re.fullmatch(r'[0-9]+(?:\.[0-9]+)+', version):
        raise ValueError('Version must be dotted numeric without a leading v')
    directory = Path(directory)
    expected = {f'{PLUGIN_ID}_{version}_{goos}_{goarch}.zip': (goos, goarch, extension) for (goos, goarch), extension in PLATFORMS.items()}
    present = {file.name for file in directory.glob('*.zip')}
    missing, unexpected = set(expected) - present, present - set(expected)
    if missing or unexpected:
        raise ValueError(f'Incomplete release matrix; missing={sorted(missing)}, unexpected={sorted(unexpected)}')
    sums = {}
    for line in (directory / 'checksums.txt').read_text(encoding='utf-8').splitlines():
        if not line.strip():
            continue
        fields = line.split(maxsplit=1)
        if len(fields) != 2 or not re.fullmatch(r'[0-9a-fA-F]{64}', fields[0]):
            raise ValueError('Invalid checksums.txt entry')
        name = fields[1].strip().removeprefix('*')
        if name in sums:
            raise ValueError(f'Duplicate checksum: {name}')
        sums[name] = fields[0].lower()
    if set(sums) != set(expected):
        raise ValueError('checksums.txt must cover exactly the six required archives')
    results = []
    for name, (goos, goarch, extension) in expected.items():
        archive = directory / name
        digest = hashlib.sha256(archive.read_bytes()).hexdigest()
        if digest != sums[name]:
            raise ValueError(f'Checksum mismatch: {name}')
        with zipfile.ZipFile(archive) as zipped:
            members = zipped.infolist()
            if len(members) != 1 or members[0].filename != f'{PLUGIN_ID}.{extension}' or stat.S_ISLNK(members[0].external_attr >> 16):
                raise ValueError(f'Expected one root-level library in {name}')
            data = zipped.read(members[0])
        verify_architecture(data, goos, goarch)
        results.append({'platform': f'{goos}/{goarch}', 'archive': name, 'sha256': digest})
    return results


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--version', required=True)
    parser.add_argument('--directory', type=Path, required=True)
    args = parser.parse_args()
    try:
        results = verify_release(args.directory, args.version)
    except (ValueError, OSError, zipfile.BadZipFile) as error:
        print(f'Release verification failed: {error}', file=sys.stderr)
        return 1
    print(f'Verified all {len(results)} required platform archives, checksums and binary architectures.')
    return 0


if __name__ == '__main__':
    sys.exit(main())
