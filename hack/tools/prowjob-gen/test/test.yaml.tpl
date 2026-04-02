{{ .branch }}
{{ ReplaceAll .branch "." "-" }}
{{ TrimPrefix "foobar" "foo" }}
{{ TrimPrefix "foobar" "bar" }}
{{ (last $.config.Upgrades).From }}
{{ (last $.config.Other.foo).from }}