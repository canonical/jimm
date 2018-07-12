package admincmd

import (
	"github.com/juju/httprequest"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"
)

func BakeryDoer(c *httpbakery.Client) httprequest.Doer {
	return bakeryDoer{c}
}
