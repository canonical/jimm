package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"

	"github.com/CanonicalLtd/jimm/internal/rpc"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	jujuparams "github.com/juju/juju/rpc/params"
	"gopkg.in/errgo.v1"
)

// A ResponseHandler is a function that is used by OpenWebBrowser to
// perform further processing with a response. A ResponseHandler should
// parse the response to determine the next action, close the body of the
// incoming response and perform queries in order to return the final
// response to the caller. The final response should not have its body
// closed.
type responseHandler func(*http.Client, *http.Response) (*http.Response, error)

// OpenWebBrowser returns a function that simulates opening a web browser
// to complete a login. This function only returns a non-nil error to its
// caller if there is an error initialising the client. If rh is non-nil
// it will be called with the *http.Response that was received by the
// client. This handler should arrange for any required further
// processing and return the result.
func openWebBrowser(rh responseHandler) func(u *url.URL) error {
	return func(u *url.URL) error {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return errgo.Mask(err)
		}
		client := &http.Client{
			Jar: jar,
		}
		resp, err := client.Get(u.String())
		if err != nil {
			fmt.Printf("error getting login URL %s: %s\n", u.String(), err)
			return nil
		}
		if rh != nil {
			resp, err = rh(client, resp)
			if err != nil {
				fmt.Printf("error handling login response: %v\n", err)
				return nil
			}
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			buf, _ := ioutil.ReadAll(resp.Body)
			fmt.Printf("interaction returned error status (%s): %s\n", resp.Status, buf)
		}
		return nil
	}
}

// SelectInteractiveLogin is a ResponseHandler that processes the list of
// login methods in the incoming response and performs a GET on that URL.
// If rh is non-nil it will be used to further process the response
// before returning to the caller.
func selectInteractiveLogin(rh responseHandler) responseHandler {
	return func(client *http.Client, resp *http.Response) (*http.Response, error) {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, errgo.Newf("unexpected status %q", resp.Status)
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, errgo.Mask(err)
		}

		// The body, as specified by the
		// authenticationRequiredTemplate, will be a list of
		// interactive login URLs, one on each line. Choose the
		// first valid one.

		parts := bytes.Split(body, []byte("\n"))
		lurl := ""
		for _, p := range parts {
			if len(p) == 0 {
				continue
			}
			s := string(p)
			if strings.Contains(s, "http://") {
				s1 := strings.Split(s, "href=\"")
				s2 := strings.Split(s1[1], "\"")
				lurl = s2[0]
				break
			}
		}
		if lurl == "" {
			return nil, errgo.New("login returned no URLs")
		}
		resp, err = client.Get(lurl)
		if err != nil {
			return resp, errgo.Mask(err)
		}
		if rh != nil {
			resp, err = rh(client, resp)
		}
		return resp, errgo.Mask(err, errgo.Any)
	}
}

// LoginFormAction gets the action parameter (POST URL) of a login form.
func loginFormAction(resp *http.Response) (string, error) {
	form := bufio.NewScanner(resp.Body)
	for form.Scan() {
		if strings.Contains(form.Text(), "http://") {
			s1 := strings.Split(form.Text(), "action=\"")
			s2 := strings.Split(s1[1], "\"")
			return s2[0], nil
		}

	}
	return resp.Request.URL.String(), nil
}

// PostLoginForm returns a ResponseHandler that can be passed to
// OpenWebBrowser which will complete a login form with the given
// Username and Password, and return the result.
func postLoginForm(username, password string) responseHandler {
	return func(client *http.Client, resp *http.Response) (*http.Response, error) {
		defer resp.Body.Close()
		purl, err := loginFormAction(resp)
		if err != nil {
			return nil, errgo.Mask(err)
		}
		resp, err = client.PostForm(purl, url.Values{
			"username": {username},
			"password": {password},
		})
		return resp, errgo.Mask(err, errgo.Any)
	}
}

// authy
// A simple script to gather an initial macaroon on start to authenticate against JIMM with
// in a local environment.
func main() {
	rpcclient, err := (&rpc.Dialer{}).Dial(context.TODO(), "ws://0.0.0.0:17070/api", false)
	if err != nil {
		log.Fatal("failed to dial controller:", err)
	}
	respres := jujuparams.LoginResult{}

	err = rpcclient.Call(context.Background(), "Admin", 3, "", "Login", nil, &respres)
	if err != nil {
		fmt.Println("failed to hit login facade:", err)
	}

	macaroon := respres.BakeryDischargeRequired

	bakeryclient := httpbakery.NewClient()

	bakeryclient.AddInteractor(httpbakery.WebBrowserInteractor{
		OpenWebBrowser: openWebBrowser(selectInteractiveLogin(postLoginForm("jimm", "jimm"))),
	})

	discharged, err := bakeryclient.DischargeAll(context.TODO(), macaroon)
	if err != nil {
		fmt.Println("failed to discharge macaroons:", err)
	}

	maccaroonieswoonies, _ := json.Marshal(discharged)
	fmt.Println()
	fmt.Println(string(maccaroonieswoonies))
	fmt.Println()
	fmt.Println("Copy the macaroons, head to the JIMMY postman collection and update your local collection variable for API_AUTH.")
}
