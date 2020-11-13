package dropin

import (
	. "github.com/onsi/gomega"
	"testing"
)

func TestNoProxyUrl(t *testing.T) {
	g := NewWithT(t)

	httpProxy := HttpProxy{}
	out, err := httpProxy.Parse()

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(out)).To(Equal("[Service]\n"))
}

func TestWithProxyUrl(t *testing.T) {
	g := NewWithT(t)

	httpProxy := HttpProxy{
		HttpProxy:  "proxy1.infra.local:3128",
		HttpsProxy: "proxy2.infra.local:3128",
	}
	out, err := httpProxy.Parse()

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(out)).To(Equal(`[Service]
Environment="HTTP_PROXY=proxy1.infra.local:3128"
Environment="HTTPS_PROXY=proxy2.infra.local:3128"
`))
}

func TestWithNoProxyList(t *testing.T) {
	g := NewWithT(t)

	httpProxy := HttpProxy{
		HttpProxy:  "proxy1.infra.local:3128",
		HttpsProxy: "proxy2.infra.local:3128",
		NoProxy:    []string{"url1", "url2"},
	}
	out, err := httpProxy.Parse()

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(out)).To(Equal(`[Service]
Environment="HTTP_PROXY=proxy1.infra.local:3128"
Environment="HTTPS_PROXY=proxy2.infra.local:3128"
Environment="NO_PROXY=url1,url2"
`))
}
