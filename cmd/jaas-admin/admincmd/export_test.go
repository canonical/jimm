package admincmd

import (
	"github.com/juju/httprequest"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

func BakeryDoer(c *httpbakery.Client) httprequest.Doer {
	return bakeryDoer{c}
}
