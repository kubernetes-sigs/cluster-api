{{ .branch }}
{{ ReplaceAll .branch "." "-" }}
{{ TrimPrefix "foobar" "foo" }}
{{ TrimPrefix "foobar" "bar" }}
{{ (last $.config.Upgrades).From }}
{{ (last $.config.Other.foo).from }}
{{ cronBucket "periodic-e2e" 2 3 60 }}
{{ cronBucket "periodic-e2e" 2 3 60 }}
{{ cronBucket "periodic-e2e" 2 3 60 }}
{{ cronBucket "periodic-e2e" 2 3 60 }}