import unittest


class CommandCleaningTests(unittest.TestCase):
    def test_joins_explicit_backtick_lines_into_one_logical_command(self):
        from aetheris.command_cleaning import assemble_logical_commands

        commands = assemble_logical_commands([
            "Start-Process powershell.exe -ArgumentList `",
            '  "-NoExit", "-Command", `',
            '  "Set-Location E:\\code"',
        ], syntax_checker=lambda _: True)

        self.assertEqual(len(commands), 1)
        self.assertEqual(commands[0].line_start, 1)
        self.assertEqual(commands[0].line_end, 3)
        self.assertEqual(commands[0].merge_method, "explicit_backtick_join")
        self.assertNotIn("`", commands[0].command)

    def test_reorders_orphan_parameter_before_primary_command(self):
        from aetheris.command_cleaning import assemble_logical_commands

        commands = assemble_logical_commands([
            "  -Recurse -Filter Microsoft.DriverKit.Build.Tasks.17.0.dll",
            'Get-ChildItem "D:\\Program Files (x86)\\Windows Kits\\10\\build"',
        ], syntax_checker=lambda _: True)

        self.assertEqual(len(commands), 1)
        self.assertEqual(commands[0].command, 'Get-ChildItem "D:\\Program Files (x86)\\Windows Kits\\10\\build" -Recurse -Filter Microsoft.DriverKit.Build.Tasks.17.0.dll')
        self.assertEqual(commands[0].merge_method, "inferred_parameter_join")
        self.assertEqual(commands[0].reason_codes, ["orphan_parameter_fragment", "inferred_reordered_join"])

    def test_joins_orphan_parameter_after_primary_command(self):
        from aetheris.command_cleaning import assemble_logical_commands

        commands = assemble_logical_commands([
            'Get-ChildItem "D:\\Program Files (x86)\\Windows Kits\\10\\build"',
            "-Recurse -Filter Microsoft.DriverKit.Build.Tasks.17.0.dll",
        ], syntax_checker=lambda _: True)

        self.assertEqual(len(commands), 1)
        self.assertEqual(commands[0].merge_method, "inferred_parameter_join")
        self.assertEqual(commands[0].line_start, 1)
        self.assertEqual(commands[0].line_end, 2)

    def test_even_trailing_backticks_do_not_consume_next_fragment(self):
        from aetheris.command_cleaning import assemble_logical_commands

        commands = assemble_logical_commands([
            'Get-ChildItem "C:\\Program Files (x86)\\Windows Kits\\10\\build" ``',
            '-Recurse -Filter Microsoft.DriverKit.Build.Tasks.17.0.dll',
            'Get-ChildItem "D:\\Program Files (x86)\\Windows Kits\\10\\build"',
        ], syntax_checker=lambda _: True)

        self.assertEqual(len(commands), 2)
        self.assertEqual(commands[1].merge_method, 'inferred_parameter_join')
        self.assertTrue(commands[1].command.startswith('Get-ChildItem "D:'))

    def test_quarantines_parameter_fragment_when_no_valid_join_exists(self):
        from aetheris.command_cleaning import assemble_logical_commands

        commands = assemble_logical_commands(["-Recurse -Filter *.dll"], syntax_checker=lambda _: False)

        self.assertEqual(commands[0].quality_state, "quarantined")
        self.assertTrue(commands[0].excluded_from_effectiveness)
        self.assertIn("orphan_parameter_fragment", commands[0].reason_codes)


if __name__ == "__main__":
    unittest.main()
