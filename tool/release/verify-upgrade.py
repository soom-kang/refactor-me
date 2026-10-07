#!/usr/bin/env python3
"""Optional provider-free upgrade check for the disposable public-install runner."""
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile


def main():
    if len(sys.argv) != 2 or not re.fullmatch(r'[0-9a-f]{40}', sys.argv[1]):
        raise SystemExit('usage: verify-upgrade.py <prior-tap-commit-sha>')
    if os.environ.get('GITHUB_ACTIONS') != 'true':
        raise SystemExit('upgrade verification requires the disposable GitHub Actions runner')
    env = dict(os.environ, HOMEBREW_NO_AUTO_UPDATE='1')

    def run(*args):
        return subprocess.check_output(args, env=env)

    formula = 'soom-kang/refactor-me/refactor-me'
    if b'refactor-me' in run('brew', 'list', '--formula').splitlines():
        raise SystemExit('upgrade verification requires no existing refactor-me installation')
    tap = Path(run('brew', '--repository', 'soom-kang/refactor-me').decode().strip())
    if run('git', '-C', str(tap), 'status', '--porcelain').strip():
        raise SystemExit('upgrade verification requires a clean tap checkout')
    current_commit = run('git', '-C', str(tap), 'rev-parse', 'HEAD').decode().strip()
    prior_commit = sys.argv[1]
    run('git', '-C', str(tap), 'fetch', '--no-tags', 'origin', prior_commit)
    formula_path = tap / 'Formula/refactor-me.rb'
    current_formula = formula_path.read_bytes()
    prior_formula = run('git', '-C', str(tap), 'show', prior_commit + ':Formula/refactor-me.rb')
    if prior_formula == current_formula:
        raise SystemExit('prior formula is identical to the current formula')
    try:
        formula_path.write_bytes(prior_formula)
        subprocess.run(['brew', 'install', formula], env=env, check=True)
        subprocess.run(['brew', 'test', formula], env=env, check=True)
        before_version = json.loads(run('refactor-me', 'version', '--json'))
    finally:
        formula_path.write_bytes(current_formula)

    with tempfile.TemporaryDirectory(prefix='refactor-upgrade-', dir=os.environ['RUNNER_TEMP']) as directory:
        repo = Path(directory) / 'project'
        run('git', 'init', '-q', str(repo))
        run('refactor-me', 'init', '--repo', str(repo))
        state = repo / '.refactor'
        config_path = state / 'config.json'
        config = json.loads(config_path.read_bytes())
        assert config['schema_version'] == 2
        config['policy']['max_cycles'] = 1
        config_path.write_text(json.dumps(config, indent=4) + '\n')
        report_dir = state / 'runs/upgrade-fixture'
        report_dir.mkdir()
        report_path = report_dir / 'report.json'
        # Seeded compatibility input, not a report produced by a provider run.
        report = {'schemaVersion': 3, 'runId': 'upgrade-fixture', 'status': 'NO_CHANGES',
                  'toolVersion': before_version['version'], 'commits': [], 'skipped': [],
                  'reason': 'Seeded provider-free upgrade fixture'}
        report_path.write_text(json.dumps(report, indent=2) + '\n')
        (state / 'last-run.json').write_text(json.dumps({'schemaVersion': 1, 'runDir': str(report_dir)}) + '\n')

        def snapshot():
            return {p.relative_to(state): p.read_bytes() for p in state.rglob('*') if p.is_file()}

        before = snapshot()
        assert run('refactor-me', 'report', '--json', '--repo', str(repo)) == report_path.read_bytes()
        assert snapshot() == before, 'prior CLI changed saved project state'
        subprocess.run(['brew', 'upgrade', formula], env=env, check=True)
        subprocess.run(['brew', 'test', formula], env=env, check=True)
        after_version = json.loads(run('refactor-me', 'version', '--json'))
        assert after_version['version'] != before_version['version'], 'upgrade did not change the installed version'
        run('refactor-me', 'init', '--repo', str(repo))
        assert run('refactor-me', 'report', '--json', '--repo', str(repo)) == before[Path('runs/upgrade-fixture/report.json')]
        assert snapshot() == before, 'upgrade/init/report changed saved project state'

        report['schemaVersion'] = 2
        report_path.write_text(json.dumps(report, indent=2) + '\n')
        unsupported = snapshot()
        rejected = subprocess.run(['refactor-me', 'report', '--json', '--repo', str(repo)], env=env, capture_output=True)
        assert rejected.returncode != 0 and not rejected.stdout and b'unsupported schemaVersion' in rejected.stderr, rejected
        assert snapshot() == unsupported, 'unsupported report was changed or removed'
        print(json.dumps({'upgradeFromFormula': prior_commit, 'upgradeToFormula': current_commit,
                          'before': before_version, 'after': after_version,
                          'statePreserved': True, 'unsupportedReportRejected': True,
                          'reportSource': 'seeded schema-3 compatibility fixture'}, indent=2))


if __name__ == '__main__':
    main()
