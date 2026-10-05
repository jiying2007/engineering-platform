#!/usr/bin/env python3
"""Offline contract regression. No systemd unit or privilege change occurs."""
import configparser
from pathlib import Path
import os
import tempfile
import unittest
from unittest import mock
import systemd_lifecycle as lifecycle

ROOT = Path(__file__).resolve().parents[1]
TEMPLATES = ROOT / 'examples/production/systemd'


class LifecycleContracts(unittest.TestCase):
    def setUp(self):
        self.raw = (TEMPLATES / 'engineering-publisher.service').read_text()

    def test_all_four_existing_templates_have_explicit_bounded_policy(self):
        self.assertEqual(sorted(p.stem for p in TEMPLATES.glob('*.service')), sorted('engineering-' + role for role in lifecycle.ROLES))
        for role in lifecycle.ROLES:
            with self.subTest(role=role):
                p = lifecycle.parse_unit((TEMPLATES / ('engineering-' + role + '.service')).read_text())
                self.assertNotIn('ExecStartPre', p['Service'])
                self.assertNotIn('--codex-config', p['Service']['ExecStart'])
                self.assertNotIn('RuntimeMaxSec', p['Service'])
                if role == 'worker-admission':
                    self.assertIn('--admission-only', p['Service']['ExecStart'])
                if role == 'worker-preparation':
                    self.assertIn('--prepare-only', p['Service']['ExecStart'])
                if role == 'control-plane':
                    self.assertTrue(p['Service']['ExecStart'].endswith('--production'))

    def test_old_unbounded_and_weakened_policies_reject(self):
        for old, new in [('StartLimitBurst=3', 'StartLimitBurst=0'), ('StartLimitIntervalSec=60s', 'StartLimitIntervalSec=0'), ('KillMode=control-group', 'KillMode=process'), ('KillSignal=SIGTERM', 'KillSignal=SIGKILL'), ('SendSIGKILL=yes', 'SendSIGKILL=no'), ('NoNewPrivileges=true', 'NoNewPrivileges=false'), ('RestartSec=5s', 'RestartSec=0'), ('Restart=on-failure', 'Restart=always'), ('ProtectSystem=strict', 'ProtectSystem=false')]:
            with self.subTest(old=old), self.assertRaises(ValueError):
                lifecycle.parse_unit(self.raw.replace(old, new))
        with self.assertRaises(ValueError):
            lifecycle.parse_unit(self.raw.replace('StartLimitBurst=3\n', ''))

    def test_mapping_retains_all_original_service_properties(self):
        root = Path('/run/ep-sysd-1-1-1234')
        original = lifecycle.parse_unit(self.raw)['Service']
        props = dict(s[len('--property='):].split('=', 1) for s in lifecycle.publisher_properties(self.raw, root, 1000, 1001))
        for key in original:
            if key not in {'ExecStart', 'User', 'Group', 'EnvironmentFile', 'ReadOnlyPaths'}:
                self.assertEqual(props[key], original[key])
        self.assertEqual(props['User'], '1000')
        self.assertEqual(props['Group'], '1001')
        self.assertEqual(props['EnvironmentFile'], str(root/'publisher.env'))
        self.assertEqual(props['ReadOnlyPaths'], str(root/'artifacts'))
        self.assertNotIn('ExecStart', props)
        self.assertNotIn('Wants', props)
        self.assertNotIn('After', props)
        for key,value in lifecycle.RATE_POLICY.items():
            self.assertEqual(props[key], value)

    def test_unreviewed_fields_and_role_drift_reject(self):
        for old, new in [('ExecStart=/opt/engineering-platform/bin/publisher-service', 'ExecStart=/bin/sh'), ('User=engineering-publisher', 'User=root'), ('Group=engineering-platform', 'Group=root'), ('EnvironmentFile=/etc/engineering-platform/publisher.env', 'EnvironmentFile=-/etc/private'), ('[Service]', '[Service]\nExecStartPre=/bin/true'), ('[Unit]', '[Unit]\nOnFailure=other.service')]:
            with self.subTest(old=old), self.assertRaises(ValueError):
                lifecycle.publisher_properties(self.raw.replace(old,new), Path('/run/test'), 1000, 1000)
        for changed in (self.raw.replace('Restart=on-failure', 'Restart=on-failure\nRestart=always'), self.raw + '\n[Other]\nKey=value\n', self.raw.replace('[Service]', '[Service]\nPrivateTmp=true')):
            with self.assertRaises((ValueError, configparser.Error)):
                lifecycle.parse_unit(changed)

    def test_no_implicit_systemd_or_privileged_operation(self):
        with tempfile.TemporaryDirectory() as tmp, mock.patch.dict(os.environ, {}, clear=True), mock.patch.object(lifecycle, 'command') as cmd:
            with self.assertRaisesRegex(ValueError, 'explicit ephemeral'):
                lifecycle.run_suite(Path(tmp), 'a'*40, Path(tmp), 'ep-sysd-1-1')
            cmd.assert_not_called()
        for group in ('/', '/system.slice/other.service', '/../ep-sysd-1.service', '/system.slice/ep-sysd-../x.service'):
            with self.assertRaises(ValueError):
                lifecycle.cgroup_empty(group)

    def test_ci_scope_requires_manager_and_cleans_only_its_prefix(self):
        script = (ROOT/'scripts/ci-systemd-lifecycle.sh').read_text()
        for text in ('/run/systemd/private', 'sudo -n true', 'trap cleanup EXIT', 'EP_SYSTEMD_INTEGRATION=1', '--work-root "$root"', 'systemd_lifecycle.py', 'git rev-parse HEAD', 'reset-failed'):
            self.assertIn(text, script)
        for forbidden in ('daemon-reload', '/etc/systemd', 'sysctl -w', 'systemctl enable', 'systemctl restart engineering-'):
            self.assertNotIn(forbidden, script)
        ci = (ROOT/'.github/workflows/ci.yml').read_text()
        native = (ROOT/'cmd/control-plane/systemd_integration_test.go').read_text()
        self.assertIn('func TestOfflineCommandSystemdLifecycle', native)
        self.assertIn('EP_SANDBOX_INTEGRATION', native)
        self.assertIn('ci-systemd-lifecycle.sh', native)
        self.assertIn('TestRealOffline|TestOfflineCommand', ci)
        self.assertNotIn('continue-on-error: true', ci)


if __name__ == '__main__':
    unittest.main()
