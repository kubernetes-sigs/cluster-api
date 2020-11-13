package dropin

import (
	"bytes"
	"text/template"
)

type HttpProxy struct {
	HttpProxy  string
	HttpsProxy string
	NoProxy    []string
}

const (
	httpProxyTemplate = `[Service]
{{- if ne .HttpProxy "" }}
Environment="HTTP_PROXY={{.HttpProxy}}"
{{- end }}
{{- if ne .HttpsProxy "" }}
Environment="HTTPS_PROXY={{.HttpsProxy}}"
{{- end }}
{{- if .NoProxy }}
Environment="NO_PROXY=
    {{- range $index, $item := .NoProxy -}}
        {{- if $index -}},{{- end -}}
        {{- $item -}}
    {{- end -}}"
{{- end }}
`
)

func (p *HttpProxy) Parse() ([]byte, error) {
	t, err := template.New("proxy").Parse(httpProxyTemplate)
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	if err := t.Execute(&out, p); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
