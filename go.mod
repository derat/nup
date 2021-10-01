module github.com/derat/nup

go 1.12

// As of 20210930, Google Cloud Build seems to have problems downloading
// gopkg.in dependencies, resulting in crap like the following when
// deploying:
//
//   go: gopkg.in/errgo.v2@v2.1.0: unknown revision v2.1.0
//   go: gopkg.in/yaml.v2@v2.2.2: unknown revision v2.2.2
//
// I see that gopkg.in uses a Let's Encrypt certificate, so I'm going to guess
// that the DST Root CA X3 expiration today is the cause of the issue (since it
// also broke GCP Monitoring and Android's DNS-over-TLS implementation):
// https://letsencrypt.org/docs/dst-root-ca-x3-expiration-september-2021/
//
// Rewriting these gopkg.in dependencies to the underlying repos seems to
// function as a very hacky workaround.
replace gopkg.in/check.v1 => github.com/go-check/check v1.0.0-20180628173108-788fd7840127
replace gopkg.in/errgo.v2 => github.com/go-errgo/errgo v2.1.0
replace gopkg.in/yaml.v2 => github.com/go-yaml/yaml v2.2.2

require (
	cloud.google.com/go/storage v1.6.0
	github.com/derat/taglib-go v0.0.0-20200408183415-49d1875d1328
	golang.org/x/image v0.0.0-20190802002840-cff245a6509b
	google.golang.org/api v0.18.0
	google.golang.org/appengine v1.6.5
)
