"""Exercise installer side effects with local archives and disposable profiles."""
import io
import os
import pathlib
import subprocess
import tarfile
import tempfile
import unittest

INSTALLER = pathlib.Path(__file__).resolve().parents[1] / 'install.sh'


class InstallTest(unittest.TestCase):
    def run_installer(self, root, *, on_path, no_modify=False, shadow_existing=False):
        tools = root / 'tools'
        destination = root / 'install [fixture]'
        profiles = root / 'profiles'
        for directory in [tools, destination, profiles]:
            directory.mkdir()
        archive = root / 'archive.tar.gz'
        binary = b'#!/bin/sh\nprintf "fixture-version\\n"\n'
        with tarfile.open(archive, 'w:gz') as output:
            member = tarfile.TarInfo('flywork-v26.10.17-fixture/flywork')
            member.size, member.mode = len(binary), 0o755
            output.addfile(member, io.BytesIO(binary))
        if shadow_existing:
            old = tools / 'flywork'
            old.write_text('#!/bin/sh\nprintf "old-version\\n"\n')
            old.chmod(0o755)
        curl = tools / 'curl'
        curl.write_text('#!/usr/bin/env python3\nimport shutil, sys\na = sys.argv[1:]\nshutil.copyfile(' + repr(str(archive)) + ', a[a.index("-o") + 1])\n')
        curl.chmod(0o755)
        env = os.environ.copy()
        env['HOME'] = str(profiles)
        env['SHELL'] = '/bin/zsh'
        env['FLYWORK_VERSION'] = 'v26.10.17'
        env['FLYWORK_INSTALL_DIR'] = str(destination)
        if no_modify:
            env['FLYWORK_NO_MODIFY_PATH'] = '1'
        components = [str(tools)] + ([str(destination)] if on_path else []) + [env['PATH']]
        components += [f'/unused/path-component-{index:04d}' for index in range(3500)]
        env['PATH'] = ':'.join(components)
        result = subprocess.run(['bash', str(INSTALLER)], env=env, text=True, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue((destination / 'flywork').is_file())
        self.assertIn('fixture-version', result.stdout, 'installer verified a different binary on PATH')
        return profiles, destination

    def test_existing_isolated_path_never_edits_profiles(self):
        with tempfile.TemporaryDirectory() as directory:
            profiles, _ = self.run_installer(pathlib.Path(directory), on_path=True)
            self.assertEqual(list(profiles.iterdir()), [], 'installer changed a profile despite destination already being on PATH')

    def test_no_modify_flag_never_edits_profiles_with_missing_path(self):
        with tempfile.TemporaryDirectory() as directory:
            profiles, _ = self.run_installer(pathlib.Path(directory), on_path=False, no_modify=True)
            self.assertEqual(list(profiles.iterdir()), [], 'opt-out changed a shell profile')


    def test_verifies_the_selected_binary_when_an_older_copy_is_first_on_path(self):
        with tempfile.TemporaryDirectory() as directory:
            self.run_installer(pathlib.Path(directory), on_path=True, shadow_existing=True)

    def test_normal_install_adds_missing_path_once(self):
        with tempfile.TemporaryDirectory() as directory:
            profiles, destination = self.run_installer(pathlib.Path(directory), on_path=False)
            content = (profiles / '.zshrc').read_text()
            self.assertEqual(content.count(str(destination)), 1)


if __name__ == '__main__':
    unittest.main()
