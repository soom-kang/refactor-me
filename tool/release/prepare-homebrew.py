#!/usr/bin/env python3
"""Prepare a source archive and formula; never publish or modify a tap."""
import argparse
import gzip
import hashlib
import io
import json
from pathlib import Path
import re
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parents[2]


def git(*args):
    return subprocess.check_output(['git', '-C', str(ROOT), *args], text=True).strip()


def prepare(output, candidate=False):
    version = (ROOT / 'tool/RELEASE_VERSION').read_text().strip()
    if not re.fullmatch(r'\d+\.\d+\.\d+-beta\.[1-9]\d*', version):
        raise ValueError('invalid release version')
    commit = git('rev-parse', 'HEAD')
    if not re.fullmatch(r'[0-9a-f]{40}', commit):
        raise ValueError('invalid commit')
    dirty = bool(git('status', '--porcelain', '--untracked-files=normal'))
    if dirty and not candidate:
        raise ValueError('dirty checkout: use --candidate for local, non-publishable validation')
    if not candidate and git('rev-parse', f'v{version}^{{commit}}') != commit:
        raise ValueError('release tag does not identify HEAD')
    if f'## {version}\n' not in (ROOT / 'tool/CHANGELOG.md').read_text():
        raise ValueError('missing changelog entry')
    output = Path(output).resolve()
    output.mkdir(parents=True, exist_ok=True)
    archive = output / f'refactor-me_{version}_source.tar.gz'
    formula = output / 'refactor-me.rb'
    if any(p.exists() for p in [archive, formula, output / 'SHA256SUMS', output / 'source.json']):
        raise ValueError('output already exists; choose a fresh directory')
    # Git supplies the allowlist: ignored local files must never enter a release.
    if candidate:
        listing = subprocess.check_output(['git', '-C', str(ROOT), 'ls-files', '-z', '--cached', '--others', '--exclude-standard', 'tool/go'])
        names = sorted(set(listing.decode().split('\0')) - {''})
    else:
        listing = subprocess.check_output(['git', '-C', str(ROOT), 'ls-tree', '-r', '--name-only', '-z', commit, '--', 'tool/go'])
        names = listing.decode().split('\0')
    names = ['LICENSE', 'tool/RELEASE_VERSION'] + [n for n in names if n and Path(n).suffix in {'.go', '.txt', '.mod', '.sum', '.json'}]
    metadata = {'version': version, 'commit': commit, 'dirty': dirty, 'candidate': candidate}
    raw = io.BytesIO()
    with tarfile.open(fileobj=raw, mode='w', format=tarfile.PAX_FORMAT) as tar:
        for name in names:
            path = ROOT / name
            if candidate and not path.exists():
                continue  # Locally removed tracked files are absent from candidates.
            if path.is_symlink():
                raise ValueError(f'source symlink is not permitted: {name}')
            body = path.read_bytes() if candidate else subprocess.check_output(['git', '-C', str(ROOT), 'show', f'{commit}:{name}'])
            info = tarfile.TarInfo(f'refactor-me-{version}/{name}')
            info.size = len(body)
            info.mode = 0o644
            tar.addfile(info, io.BytesIO(body))
        body = (json.dumps(metadata, sort_keys=True, indent=2) + '\n').encode()
        info = tarfile.TarInfo(f'refactor-me-{version}/BUILD-INFO.json')
        info.size = len(body)
        info.mode = 0o644
        tar.addfile(info, io.BytesIO(body))
    archive.write_bytes(gzip.compress(raw.getvalue(), mtime=0))
    sha = hashlib.sha256(archive.read_bytes()).hexdigest()
    url = archive.as_uri() if candidate else f'https://github.com/soom-kang/refactor-me/releases/download/v{version}/{archive.name}'
    template = (ROOT / 'tool/release/homebrew/refactor-me.rb.in').read_text()
    for key, value in {'VERSION': version, 'COMMIT': commit, 'SHA256': sha, 'URL': url}.items():
        template = template.replace('@' + key + '@', value)
    formula.write_text(template)
    (output / 'SHA256SUMS').write_text(f'{sha}  {archive.name}\n')
    metadata.update(sha256=sha, url=url, archive=archive.name)
    (output / 'source.json').write_text(json.dumps(metadata, indent=2) + '\n')
    return metadata


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('output')
    parser.add_argument('--candidate', action='store_true', help='non-publishable local archive and file URL formula')
    args = parser.parse_args()
    try:
        print(json.dumps(prepare(args.output, args.candidate), indent=2))
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        parser.exit(2, f'{error}\n')
