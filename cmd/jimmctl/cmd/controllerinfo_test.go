// Copyright 2024 Canonical.

package cmd_test

import (
	"os"
	"path"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/juju/jujuclient"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/cmd/jimmctl/cmd"
	"github.com/canonical/jimm/v3/internal/testutils/cmdtest"
)

type controllerInfoSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&controllerInfoSuite{})

func (s *controllerInfoSuite) TestControllerInfo(c *gc.C) {
	store := s.ClientStore()
	store.Controllers["controller-1"] = jujuclient.ControllerDetails{
		ControllerUUID: "982b16d9-a945-4762-b684-fd4fd885aa11",
		APIEndpoints:   []string{"127.0.0.1:17070"},
		PublicDNSName:  "controller1.example.com",
		CACert: `-----BEGIN CERTIFICATE-----
  MIID/jCCAmagAwIBAgIVANxsMrzsXrdpjjUoxWQm1RCkmWcqMA0GCSqGSIb3DQEB
  CwUAMCYxDTALBgNVBAoTBEp1anUxFTATBgNVBAMTDGp1anUgdGVzdGluZzAeFw0y
  MDA0MDgwNTI3NTBaFw0zMDA0MDgwNTMyNTBaMCYxDTALBgNVBAoTBEp1anUxFTAT
  BgNVBAMTDGp1anUgdGVzdGluZzCCAaIwDQYJKoZIhvcNAQEBBQADggGPADCCAYoC
  ggGBAOW4k2bmXXU3tJ8H5AsGkp8ENLJXzU4SCOCB+X0jPQRVpFtywBVD96z+l+qW
  ndGLIg5zMQTtZm71CaOw+8Sl03XU0f28Xrjf+FZCAPID1c7NBttUShbu84euFoCS
  C8yobj6JzLz7QswvkshYQ7JEZ88UXtVHqg6MGYFdu+cX/dE1jC7aHg9bus/P6bFH
  PVFcHVVxNbLy98Id1iB7i0s97H17nu9O7ZKMrAQAX6dfAELAFQVicdN3WpfwNXEj
  M2KIrqttpM8s6/57mi9UJFYGdAEDNkJr/dI506VdGLpiqTFhQK6ztfDfY08QbWk6
  iJn8vzWvNW8WthmBtEDpv+DL+a5SJSLSAIZn9sbuBBpiX+csZb66fYhKFFIUrIa5
  lrjw8yiHJ4kgsEZJSYaAn7guqmOv8clvy1E2JjsOfGycest6+1/mNdMRFgrMxdzD
  0M2yZ96zrdfF/QXpi7Hk7jFLzimuujNUpKFv7k+XObQFxeXnoFrYVkj3YT8hhYF0
  mGRkAwIDAQABoyMwITAOBgNVHQ8BAf8EBAMCAqQwDwYDVR0TAQH/BAUwAwEB/zAN
  BgkqhkiG9w0BAQsFAAOCAYEAd7GrziPRmjoK3HDF10S+5NgoKYvkOuk2jDap2Qaq
  ZFjDvrDA2tr6U0FGY+Hz+QfvtgT+YpJB5IvABvSXdq37llwKGsiSOZSrpHyTsOB0
  VcZAF3GMk1nHYMr0o1xRV2gm/ax+vUEStj0k2gTs/p57uhKcCUXR0p3PWXKcRj9a
  nVf5bdVkt6ghGs7/uEvj303raUFSf5dJ4C9RTgBK2E9/wlBYNyj5vcsshNpz8kt6
  RuARM6Boq2EwKkpRlbvImDM8ZJJLwtpijYrx3egfOSEA7Wfwgwn+B359XcosOee5
  n5BC62EjaP85cM9HCtp2DwKjNSosxja723qZPY6Z0Y7IVn3JVjgC2kWP6GViwb+v
  l9uwx9ASHPz9ilh6gpjgIifOKZYCaBSS9g8VxHpO5Izxj4vi4AX5cebDg3SzDVik
  hZuWHpuOT120okoutwuUSU9448cXLGZfoCZjjdMKXmOj8EEec1diDP4mhegYGezD
  LQRNNlaY2ajLt0paowf/Xxb8
  -----END CERTIFICATE-----`,
	}
	store.Accounts["controller-1"] = jujuclient.AccountDetails{
		User:     "test-user",
		Password: "super-secret-password",
	}
	dir, err := os.MkdirTemp("", "controller-info-test")
	c.Assert(err, gc.Equals, nil)
	defer os.RemoveAll(dir)

	fname := path.Join(dir, "test.yaml")

	_, err = cmdtesting.RunCommand(c, cmd.NewControllerInfoCommandForTesting(store), "controller-1", fname, "controller1.example.com")
	c.Assert(err, gc.IsNil)

	data, err := os.ReadFile(fname)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Matches, `api-addresses:
- 127.0.0.1:17070
name: controller-1
password: super-secret-password
public-address: controller1.example.com
username: test-user
uuid: 982b16d9-a945-4762-b684-fd4fd885aa11
`)
}

func (s *controllerInfoSuite) TestControllerInfoWithLocalFlag(c *gc.C) {
	store := s.ClientStore()
	store.Controllers["controller-1"] = jujuclient.ControllerDetails{
		ControllerUUID: "982b16d9-a945-4762-b684-fd4fd885aa11",
		APIEndpoints:   []string{"127.0.0.1:17070"},
		PublicDNSName:  "controller1.example.com",
		CACert: `-----BEGIN CERTIFICATE-----
  MIID/jCCAmagAwIBAgIVANxsMrzsXrdpjjUoxWQm1RCkmWcqMA0GCSqGSIb3DQEB
  CwUAMCYxDTALBgNVBAoTBEp1anUxFTATBgNVBAMTDGp1anUgdGVzdGluZzAeFw0y
  MDA0MDgwNTI3NTBaFw0zMDA0MDgwNTMyNTBaMCYxDTALBgNVBAoTBEp1anUxFTAT
  BgNVBAMTDGp1anUgdGVzdGluZzCCAaIwDQYJKoZIhvcNAQEBBQADggGPADCCAYoC
  ggGBAOW4k2bmXXU3tJ8H5AsGkp8ENLJXzU4SCOCB+X0jPQRVpFtywBVD96z+l+qW
  ndGLIg5zMQTtZm71CaOw+8Sl03XU0f28Xrjf+FZCAPID1c7NBttUShbu84euFoCS
  C8yobj6JzLz7QswvkshYQ7JEZ88UXtVHqg6MGYFdu+cX/dE1jC7aHg9bus/P6bFH
  PVFcHVVxNbLy98Id1iB7i0s97H17nu9O7ZKMrAQAX6dfAELAFQVicdN3WpfwNXEj
  M2KIrqttpM8s6/57mi9UJFYGdAEDNkJr/dI506VdGLpiqTFhQK6ztfDfY08QbWk6
  iJn8vzWvNW8WthmBtEDpv+DL+a5SJSLSAIZn9sbuBBpiX+csZb66fYhKFFIUrIa5
  lrjw8yiHJ4kgsEZJSYaAn7guqmOv8clvy1E2JjsOfGycest6+1/mNdMRFgrMxdzD
  0M2yZ96zrdfF/QXpi7Hk7jFLzimuujNUpKFv7k+XObQFxeXnoFrYVkj3YT8hhYF0
  mGRkAwIDAQABoyMwITAOBgNVHQ8BAf8EBAMCAqQwDwYDVR0TAQH/BAUwAwEB/zAN
  BgkqhkiG9w0BAQsFAAOCAYEAd7GrziPRmjoK3HDF10S+5NgoKYvkOuk2jDap2Qaq
  ZFjDvrDA2tr6U0FGY+Hz+QfvtgT+YpJB5IvABvSXdq37llwKGsiSOZSrpHyTsOB0
  VcZAF3GMk1nHYMr0o1xRV2gm/ax+vUEStj0k2gTs/p57uhKcCUXR0p3PWXKcRj9a
  nVf5bdVkt6ghGs7/uEvj303raUFSf5dJ4C9RTgBK2E9/wlBYNyj5vcsshNpz8kt6
  RuARM6Boq2EwKkpRlbvImDM8ZJJLwtpijYrx3egfOSEA7Wfwgwn+B359XcosOee5
  n5BC62EjaP85cM9HCtp2DwKjNSosxja723qZPY6Z0Y7IVn3JVjgC2kWP6GViwb+v
  l9uwx9ASHPz9ilh6gpjgIifOKZYCaBSS9g8VxHpO5Izxj4vi4AX5cebDg3SzDVik
  hZuWHpuOT120okoutwuUSU9448cXLGZfoCZjjdMKXmOj8EEec1diDP4mhegYGezD
  LQRNNlaY2ajLt0paowf/Xxb8
  -----END CERTIFICATE-----`,
	}
	store.Accounts["controller-1"] = jujuclient.AccountDetails{
		User:     "test-user",
		Password: "super-secret-password",
	}
	dir, err := os.MkdirTemp("", "controller-info-test")
	c.Assert(err, gc.Equals, nil)
	defer os.RemoveAll(dir)

	fname := path.Join(dir, "test.yaml")

	_, err = cmdtesting.RunCommand(c, cmd.NewControllerInfoCommandForTesting(store), "controller-1", fname, "--local")
	c.Assert(err, gc.IsNil)

	data, err := os.ReadFile(fname)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Matches, `api-addresses:
- 127.0.0.1:17070
ca-certificate: |-
  -----BEGIN CERTIFICATE-----
  MIID/jCCAmagAwIBAgIVANxsMrzsXrdpjjUoxWQm1RCkmWcqMA0GCSqGSIb3DQEB
  CwUAMCYxDTALBgNVBAoTBEp1anUxFTATBgNVBAMTDGp1anUgdGVzdGluZzAeFw0y
  MDA0MDgwNTI3NTBaFw0zMDA0MDgwNTMyNTBaMCYxDTALBgNVBAoTBEp1anUxFTAT
  BgNVBAMTDGp1anUgdGVzdGluZzCCAaIwDQYJKoZIhvcNAQEBBQADggGPADCCAYoC
  ggGBAOW4k2bmXXU3tJ8H5AsGkp8ENLJXzU4SCOCB+X0jPQRVpFtywBVD96z+l+qW
  ndGLIg5zMQTtZm71CaOw+8Sl03XU0f28Xrjf+FZCAPID1c7NBttUShbu84euFoCS
  C8yobj6JzLz7QswvkshYQ7JEZ88UXtVHqg6MGYFdu+cX/dE1jC7aHg9bus/P6bFH
  PVFcHVVxNbLy98Id1iB7i0s97H17nu9O7ZKMrAQAX6dfAELAFQVicdN3WpfwNXEj
  M2KIrqttpM8s6/57mi9UJFYGdAEDNkJr/dI506VdGLpiqTFhQK6ztfDfY08QbWk6
  iJn8vzWvNW8WthmBtEDpv+DL+a5SJSLSAIZn9sbuBBpiX+csZb66fYhKFFIUrIa5
  lrjw8yiHJ4kgsEZJSYaAn7guqmOv8clvy1E2JjsOfGycest6+1/mNdMRFgrMxdzD
  0M2yZ96zrdfF/QXpi7Hk7jFLzimuujNUpKFv7k+XObQFxeXnoFrYVkj3YT8hhYF0
  mGRkAwIDAQABoyMwITAOBgNVHQ8BAf8EBAMCAqQwDwYDVR0TAQH/BAUwAwEB/zAN
  BgkqhkiG9w0BAQsFAAOCAYEAd7GrziPRmjoK3HDF10S+5NgoKYvkOuk2jDap2Qaq
  ZFjDvrDA2tr6U0FGY+Hz+QfvtgT+YpJB5IvABvSXdq37llwKGsiSOZSrpHyTsOB0
  VcZAF3GMk1nHYMr0o1xRV2gm/ax+vUEStj0k2gTs/p57uhKcCUXR0p3PWXKcRj9a
  nVf5bdVkt6ghGs7/uEvj303raUFSf5dJ4C9RTgBK2E9/wlBYNyj5vcsshNpz8kt6
  RuARM6Boq2EwKkpRlbvImDM8ZJJLwtpijYrx3egfOSEA7Wfwgwn+B359XcosOee5
  n5BC62EjaP85cM9HCtp2DwKjNSosxja723qZPY6Z0Y7IVn3JVjgC2kWP6GViwb+v
  l9uwx9ASHPz9ilh6gpjgIifOKZYCaBSS9g8VxHpO5Izxj4vi4AX5cebDg3SzDVik
  hZuWHpuOT120okoutwuUSU9448cXLGZfoCZjjdMKXmOj8EEec1diDP4mhegYGezD
  LQRNNlaY2ajLt0paowf/Xxb8
  -----END CERTIFICATE-----
name: controller-1
password: super-secret-password
public-address: 127.0.0.1:17070
username: test-user
uuid: 982b16d9-a945-4762-b684-fd4fd885aa11
`)
}

func (s *controllerInfoSuite) TestControllerInfoMissingPublicAddressAndNoLocalFlag(c *gc.C) {
	store := s.ClientStore()
	store.Controllers["controller-1"] = jujuclient.ControllerDetails{
		ControllerUUID: "982b16d9-a945-4762-b684-fd4fd885aa11",
		APIEndpoints:   []string{"127.0.0.1:17070"},
		PublicDNSName:  "controller1.example.com",
		CACert: `-----BEGIN CERTIFICATE-----
  MIID/jCCAmagAwIBAgIVANxsMrzsXrdpjjUoxWQm1RCkmWcqMA0GCSqGSIb3DQEB
  CwUAMCYxDTALBgNVBAoTBEp1anUxFTATBgNVBAMTDGp1anUgdGVzdGluZzAeFw0y
  MDA0MDgwNTI3NTBaFw0zMDA0MDgwNTMyNTBaMCYxDTALBgNVBAoTBEp1anUxFTAT
  BgNVBAMTDGp1anUgdGVzdGluZzCCAaIwDQYJKoZIhvcNAQEBBQADggGPADCCAYoC
  ggGBAOW4k2bmXXU3tJ8H5AsGkp8ENLJXzU4SCOCB+X0jPQRVpFtywBVD96z+l+qW
  ndGLIg5zMQTtZm71CaOw+8Sl03XU0f28Xrjf+FZCAPID1c7NBttUShbu84euFoCS
  C8yobj6JzLz7QswvkshYQ7JEZ88UXtVHqg6MGYFdu+cX/dE1jC7aHg9bus/P6bFH
  PVFcHVVxNbLy98Id1iB7i0s97H17nu9O7ZKMrAQAX6dfAELAFQVicdN3WpfwNXEj
  M2KIrqttpM8s6/57mi9UJFYGdAEDNkJr/dI506VdGLpiqTFhQK6ztfDfY08QbWk6
  iJn8vzWvNW8WthmBtEDpv+DL+a5SJSLSAIZn9sbuBBpiX+csZb66fYhKFFIUrIa5
  lrjw8yiHJ4kgsEZJSYaAn7guqmOv8clvy1E2JjsOfGycest6+1/mNdMRFgrMxdzD
  0M2yZ96zrdfF/QXpi7Hk7jFLzimuujNUpKFv7k+XObQFxeXnoFrYVkj3YT8hhYF0
  mGRkAwIDAQABoyMwITAOBgNVHQ8BAf8EBAMCAqQwDwYDVR0TAQH/BAUwAwEB/zAN
  BgkqhkiG9w0BAQsFAAOCAYEAd7GrziPRmjoK3HDF10S+5NgoKYvkOuk2jDap2Qaq
  ZFjDvrDA2tr6U0FGY+Hz+QfvtgT+YpJB5IvABvSXdq37llwKGsiSOZSrpHyTsOB0
  VcZAF3GMk1nHYMr0o1xRV2gm/ax+vUEStj0k2gTs/p57uhKcCUXR0p3PWXKcRj9a
  nVf5bdVkt6ghGs7/uEvj303raUFSf5dJ4C9RTgBK2E9/wlBYNyj5vcsshNpz8kt6
  RuARM6Boq2EwKkpRlbvImDM8ZJJLwtpijYrx3egfOSEA7Wfwgwn+B359XcosOee5
  n5BC62EjaP85cM9HCtp2DwKjNSosxja723qZPY6Z0Y7IVn3JVjgC2kWP6GViwb+v
  l9uwx9ASHPz9ilh6gpjgIifOKZYCaBSS9g8VxHpO5Izxj4vi4AX5cebDg3SzDVik
  hZuWHpuOT120okoutwuUSU9448cXLGZfoCZjjdMKXmOj8EEec1diDP4mhegYGezD
  LQRNNlaY2ajLt0paowf/Xxb8
  -----END CERTIFICATE-----`,
	}
	store.Accounts["controller-1"] = jujuclient.AccountDetails{
		User:     "test-user",
		Password: "super-secret-password",
	}
	dir, err := os.MkdirTemp("", "controller-info-test")
	c.Assert(err, gc.Equals, nil)
	defer os.RemoveAll(dir)

	fname := path.Join(dir, "test.yaml")

	_, err = cmdtesting.RunCommand(c, cmd.NewControllerInfoCommandForTesting(store), "controller-1", fname)
	c.Assert(err, gc.ErrorMatches, "provide either a public address or use --local")
}

func (s *controllerInfoSuite) TestControllerInfoCannotProvideAddrAndLocalFlag(c *gc.C) {
	store := s.ClientStore()
	store.Controllers["controller-1"] = jujuclient.ControllerDetails{
		ControllerUUID: "982b16d9-a945-4762-b684-fd4fd885aa11",
		APIEndpoints:   []string{"127.0.0.1:17070"},
		PublicDNSName:  "controller1.example.com",
		CACert: `-----BEGIN CERTIFICATE-----
  MIID/jCCAmagAwIBAgIVANxsMrzsXrdpjjUoxWQm1RCkmWcqMA0GCSqGSIb3DQEB
  CwUAMCYxDTALBgNVBAoTBEp1anUxFTATBgNVBAMTDGp1anUgdGVzdGluZzAeFw0y
  MDA0MDgwNTI3NTBaFw0zMDA0MDgwNTMyNTBaMCYxDTALBgNVBAoTBEp1anUxFTAT
  BgNVBAMTDGp1anUgdGVzdGluZzCCAaIwDQYJKoZIhvcNAQEBBQADggGPADCCAYoC
  ggGBAOW4k2bmXXU3tJ8H5AsGkp8ENLJXzU4SCOCB+X0jPQRVpFtywBVD96z+l+qW
  ndGLIg5zMQTtZm71CaOw+8Sl03XU0f28Xrjf+FZCAPID1c7NBttUShbu84euFoCS
  C8yobj6JzLz7QswvkshYQ7JEZ88UXtVHqg6MGYFdu+cX/dE1jC7aHg9bus/P6bFH
  PVFcHVVxNbLy98Id1iB7i0s97H17nu9O7ZKMrAQAX6dfAELAFQVicdN3WpfwNXEj
  M2KIrqttpM8s6/57mi9UJFYGdAEDNkJr/dI506VdGLpiqTFhQK6ztfDfY08QbWk6
  iJn8vzWvNW8WthmBtEDpv+DL+a5SJSLSAIZn9sbuBBpiX+csZb66fYhKFFIUrIa5
  lrjw8yiHJ4kgsEZJSYaAn7guqmOv8clvy1E2JjsOfGycest6+1/mNdMRFgrMxdzD
  0M2yZ96zrdfF/QXpi7Hk7jFLzimuujNUpKFv7k+XObQFxeXnoFrYVkj3YT8hhYF0
  mGRkAwIDAQABoyMwITAOBgNVHQ8BAf8EBAMCAqQwDwYDVR0TAQH/BAUwAwEB/zAN
  BgkqhkiG9w0BAQsFAAOCAYEAd7GrziPRmjoK3HDF10S+5NgoKYvkOuk2jDap2Qaq
  ZFjDvrDA2tr6U0FGY+Hz+QfvtgT+YpJB5IvABvSXdq37llwKGsiSOZSrpHyTsOB0
  VcZAF3GMk1nHYMr0o1xRV2gm/ax+vUEStj0k2gTs/p57uhKcCUXR0p3PWXKcRj9a
  nVf5bdVkt6ghGs7/uEvj303raUFSf5dJ4C9RTgBK2E9/wlBYNyj5vcsshNpz8kt6
  RuARM6Boq2EwKkpRlbvImDM8ZJJLwtpijYrx3egfOSEA7Wfwgwn+B359XcosOee5
  n5BC62EjaP85cM9HCtp2DwKjNSosxja723qZPY6Z0Y7IVn3JVjgC2kWP6GViwb+v
  l9uwx9ASHPz9ilh6gpjgIifOKZYCaBSS9g8VxHpO5Izxj4vi4AX5cebDg3SzDVik
  hZuWHpuOT120okoutwuUSU9448cXLGZfoCZjjdMKXmOj8EEec1diDP4mhegYGezD
  LQRNNlaY2ajLt0paowf/Xxb8
  -----END CERTIFICATE-----`,
	}
	store.Accounts["controller-1"] = jujuclient.AccountDetails{
		User:     "test-user",
		Password: "super-secret-password",
	}
	dir, err := os.MkdirTemp("", "controller-info-test")
	c.Assert(err, gc.Equals, nil)
	defer os.RemoveAll(dir)

	fname := path.Join(dir, "test.yaml")

	_, err = cmdtesting.RunCommand(c, cmd.NewControllerInfoCommandForTesting(store), "controller-1", fname, "myaddress", "--local")
	c.Assert(err, gc.ErrorMatches, "cannot set both public address and local flag")
}

func (s *controllerInfoSuite) TestControllerInfoWithTlsFlag(c *gc.C) {
	store := s.ClientStore()
	store.Controllers["controller-1"] = jujuclient.ControllerDetails{
		ControllerUUID: "982b16d9-a945-4762-b684-fd4fd885aa11",
		APIEndpoints:   []string{"127.0.0.1:17070"},
		PublicDNSName:  "controller1.example.com",
		CACert: `-----BEGIN CERTIFICATE-----
  MIID/jCCAmagAwIBAgIVANxsMrzsXrdpjjUoxWQm1RCkmWcqMA0GCSqGSIb3DQEB
  CwUAMCYxDTALBgNVBAoTBEp1anUxFTATBgNVBAMTDGp1anUgdGVzdGluZzAeFw0y
  MDA0MDgwNTI3NTBaFw0zMDA0MDgwNTMyNTBaMCYxDTALBgNVBAoTBEp1anUxFTAT
  BgNVBAMTDGp1anUgdGVzdGluZzCCAaIwDQYJKoZIhvcNAQEBBQADggGPADCCAYoC
  ggGBAOW4k2bmXXU3tJ8H5AsGkp8ENLJXzU4SCOCB+X0jPQRVpFtywBVD96z+l+qW
  ndGLIg5zMQTtZm71CaOw+8Sl03XU0f28Xrjf+FZCAPID1c7NBttUShbu84euFoCS
  C8yobj6JzLz7QswvkshYQ7JEZ88UXtVHqg6MGYFdu+cX/dE1jC7aHg9bus/P6bFH
  PVFcHVVxNbLy98Id1iB7i0s97H17nu9O7ZKMrAQAX6dfAELAFQVicdN3WpfwNXEj
  M2KIrqttpM8s6/57mi9UJFYGdAEDNkJr/dI506VdGLpiqTFhQK6ztfDfY08QbWk6
  iJn8vzWvNW8WthmBtEDpv+DL+a5SJSLSAIZn9sbuBBpiX+csZb66fYhKFFIUrIa5
  lrjw8yiHJ4kgsEZJSYaAn7guqmOv8clvy1E2JjsOfGycest6+1/mNdMRFgrMxdzD
  0M2yZ96zrdfF/QXpi7Hk7jFLzimuujNUpKFv7k+XObQFxeXnoFrYVkj3YT8hhYF0
  mGRkAwIDAQABoyMwITAOBgNVHQ8BAf8EBAMCAqQwDwYDVR0TAQH/BAUwAwEB/zAN
  BgkqhkiG9w0BAQsFAAOCAYEAd7GrziPRmjoK3HDF10S+5NgoKYvkOuk2jDap2Qaq
  ZFjDvrDA2tr6U0FGY+Hz+QfvtgT+YpJB5IvABvSXdq37llwKGsiSOZSrpHyTsOB0
  VcZAF3GMk1nHYMr0o1xRV2gm/ax+vUEStj0k2gTs/p57uhKcCUXR0p3PWXKcRj9a
  nVf5bdVkt6ghGs7/uEvj303raUFSf5dJ4C9RTgBK2E9/wlBYNyj5vcsshNpz8kt6
  RuARM6Boq2EwKkpRlbvImDM8ZJJLwtpijYrx3egfOSEA7Wfwgwn+B359XcosOee5
  n5BC62EjaP85cM9HCtp2DwKjNSosxja723qZPY6Z0Y7IVn3JVjgC2kWP6GViwb+v
  l9uwx9ASHPz9ilh6gpjgIifOKZYCaBSS9g8VxHpO5Izxj4vi4AX5cebDg3SzDVik
  hZuWHpuOT120okoutwuUSU9448cXLGZfoCZjjdMKXmOj8EEec1diDP4mhegYGezD
  LQRNNlaY2ajLt0paowf/Xxb8
  -----END CERTIFICATE-----`,
	}
	store.Accounts["controller-1"] = jujuclient.AccountDetails{
		User:     "test-user",
		Password: "super-secret-password",
	}
	dir, err := os.MkdirTemp("", "controller-info-test")
	c.Assert(err, gc.Equals, nil)
	defer os.RemoveAll(dir)

	fname := path.Join(dir, "test.yaml")

	_, err = cmdtesting.RunCommand(c, cmd.NewControllerInfoCommandForTesting(store), "controller-1", fname, "myaddress", "--tls-hostname", "foo")
	c.Assert(err, gc.IsNil)

	data, err := os.ReadFile(fname)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Matches, `api-addresses:
- 127.0.0.1:17070
name: controller-1
password: super-secret-password
public-address: myaddress
tls-hostname: foo
username: test-user
uuid: 982b16d9-a945-4762-b684-fd4fd885aa11
`)
}
