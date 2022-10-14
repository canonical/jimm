import json
import os
import shutil
import subprocess
import tarfile
import time
import urllib.request

from charmhelpers.contrib.charmsupport.nrpe import NRPE
from charmhelpers.core import (
    hookenv,
    host,
    templating,
)
import yaml

# The port that the HTTP service listens on.
HTTP_LISTEN_PORT = 8080


def install(binary=None):
    """Install the service from the specified resource. We assume that
       the charm metadata specifies a "service" resource that is associated
       with a compressed tar archive containing the files required by the
       service, including the service binary itself.

    Parameters:
       binary - Path to the service binary within the resource tar file.
                Default is bin/<charm name>.
    """

    service = _service()
    root = _root()
    resource_path = os.path.join(root, 'service')
    if not binary:
        binary = os.path.join('bin', service)

    host.adduser(service)
    host.mkdir(root, perms=0o755)

    log_path = os.path.join('/var/log', service, service + '.log')
    if not os.path.exists(log_path):
        host.mkdir(os.path.dirname(log_path), owner='root', group=service,
                   perms=0o775)
        host.write_file(log_path, '', owner='syslog', group='adm', perms=0o640)

    rsyslog_path = os.path.join('/etc/rsyslog.d', '10-{}.conf'.format(service))
    templating.render(
        'rsyslog',
        rsyslog_path,
        {
            'log_path': log_path,
            'bin_name': os.path.basename(binary),
        },
    )
    host.service_restart('rsyslog')

    logrotate_path = os.path.join('/etc/logrotate.d', service)
    templating.render(
        'logrotate',
        logrotate_path,
        {
            'log_path': log_path,
        },
    )

    new_resource_path = resource_path + '.new'
    old_resource_path = resource_path + '.old'

    # Remove possible remnants of a failed install and
    # the previous previous resource directory.
    shutil.rmtree(new_resource_path, ignore_errors=True)
    shutil.rmtree(old_resource_path, ignore_errors=True)

    hookenv.status_set('maintenance', 'getting service resource')
    resource_file = hookenv.resource_get('service')
    if not resource_file:
        hookenv.status_set('blocked', 'waiting for service resource')
        return

    hookenv.status_set('maintenance', 'installing {}'.format(service))
    with tarfile.open(resource_file) as tf:
        tf.extractall(new_resource_path)
        # Change the owner/group of all extracted files to root/wheel.
        for name in tf.getnames():
            os.chown(os.path.join(new_resource_path, name), 0, 0)

    # Sanity check that at least the service binary exists in the
    # unarchived service resource.
    if not os.path.exists(os.path.join(new_resource_path, binary)):
        hookenv.status_set('blocked', 'no binary found in service resource')
        return

    # Move the old directory out of the way and the newly unarchived
    # directory to its destination path.
    if os.path.exists(resource_path):
        os.rename(resource_path, old_resource_path)
    os.rename(new_resource_path, resource_path)

    service_path = os.path.join(root, '{}.service'.format(service))
    context = {
        'bin_path': os.path.join(resource_path, binary),
        'config_path': _config_path(),
        'description': hookenv.metadata().get('summary'),
        'resource_path': resource_path,
        'service': service,
    }

    dashboard_file = hookenv.resource_get('dashboard')
    if _dashboard_resource_nonempty():
        new_dashboard_path = dashboard_path() + '.new'
        old_dashboard_path = dashboard_path() + '.old'
        shutil.rmtree(new_dashboard_path, ignore_errors=True)
        shutil.rmtree(old_dashboard_path, ignore_errors=True)

        hookenv.status_set('maintenance', 'installing dashboard')
        with tarfile.open(dashboard_file) as tf:
            tf.extractall(new_dashboard_path)
            # Change the owner/group of all extracted files to root/wheel.
            for name in tf.getnames():
                os.chown(os.path.join(new_dashboard_path, name), 0, 0)
        if os.path.exists(dashboard_path()):
            os.rename(dashboard_path(), old_dashboard_path)
        os.rename(new_dashboard_path, dashboard_path())

    templating.render(
        'service',
        service_path,
        context,
    )
    if not _service_enabled():
        host.service('enable', service_path)
    else:
        subprocess.check_call(('systemctl', 'daemon-reload'))

    hookenv.open_port(HTTP_LISTEN_PORT)


def stop():
    """Stop the service."""
    host.service_stop(_service())


def update_config(config):
    """Update the configuration file for the service with the
       configuration keys specified in config. If a config key has a value of
       None then that value will be removed from the configuration file.
       It reports whether the config file was changed.
    """

    """Update the config.js file for the Juju Dashboard.
    """
    if _dashboard_resource_nonempty() and os.path.exists(dashboard_path()):
        config_js_path = os.path.join(dashboard_path(), 'config.js')
        
        dashboard_context = {
            'base_controller_url': config['controller-url'],
            'base_app_url': '/dashboard/',
            'identity_provider_available': str(len(config['identity-location']) != 0).lower(),
        }
        templating.render(
            'dashboard-config',
            config_js_path,
            dashboard_context
        )

    path = _config_path()
    data = {}
    changed = True
    if os.path.exists(path):
        changed = False
        with open(path) as f:
            data = yaml.safe_load(f)

    for k in config:
        if config[k] is None:
            if k in data:
                changed = True
                del data[k]
            continue
        if data.get(k) == config[k]:
            continue
        data[k] = config[k]
        changed = True

    if not changed:
        return False

    host.write_file(
        path,
        yaml.safe_dump(data),
        group=_service(),
        perms=0o640,
    )

    return True


def update_config_and_restart(config):
    """Update the YAML configuration file for service with config
       (see update_config for how this argument is treated) and
       restart the service. The service will only be restarted if the
       configuration is changed or the service is already not yet running.
       """
    changed = update_config(config)
    if not _service_enabled():
        return
    service = _service()
    if _service_running():
        if not changed:
            return
        hookenv.status_set('maintenance',
                           'restarting {} service'.format(service))
        host.service_restart(service)
    else:
        hookenv.status_set('maintenance',
                           'starting {} service'.format(service))
        host.service_start(service)


def update_nrpe_config():
    """Update the NRPE configuration for the given service."""
    hookenv.log("updating NRPE checks")
    service = _service()
    nrpe = NRPE()
    nrpe.add_check(
        shortname=service,
        description='Check {} running'.format(service),
        check_cmd='check_http -w 2 -c 10 -I {} -p {} -u /debug/info'.format(
            hookenv.unit_private_ip(),
            HTTP_LISTEN_PORT,
        )
    )
    nrpe.write()


def update_status(failed_status=None):
    """Update the status message for the specified service.
       If failed_status is specified it will be called with the message
       when the service is determined to have failed. failed_status
       should return a tuple containing the status set and the message."""

    # Loop waiting for the service to either become available or fail. If it
    # hasn't done either after a short time stop.
    for i in range(5):
        version = _workload_version()
        if version:
            hookenv.application_version_set(version)
            hookenv.status_set('active', '')
            return
        if _service_failed():
            failed_status = failed_status or _default_failed_status
            status, msg = failed_status(_failed_msg())
            hookenv.status_set(status, msg)
            return
        # The service is not yet in a defined state sleep for a bit to enable
        # it to settle.
        time.sleep(0.2)


def _workload_version():
    """Determine the version from the running service. returns None if the
        service isn't running or it is not reporting a version."""
    if not _service_running():
        return None
    url = 'http://localhost:{}/debug/info'.format(HTTP_LISTEN_PORT)
    try:
        with urllib.request.urlopen(url) as resp:
            buf = resp.read().decode('utf-8')
            data = json.loads(buf)
    except Exception as e:
        hookenv.log('cannot get version: {}'.format(e))
        return None
    return data.get('Version', '')


def _failed_msg():
    """Determine the reason the given service is failed by checking the
       service logs."""
    cmd = ('journalctl', '-u', _service(), '-o', 'cat', '-n', '20')
    msg = ''
    try:
        out = subprocess.check_output(cmd).decode()
        for line in out.splitlines():
            if line.startswith('START'):
                msg = ''
            if line.startswith('STOP'):
                if len(line) > 5:
                    msg = line[5:]
    except subprocess.CalledProcessError:
        pass
    return msg


def _default_failed_status(msg):
    return 'blocked', msg


def _service():
    """Return the name of the service, derived from the charm name"""
    return hookenv.metadata()['name']


def _root():
    """Return the root directory for installations for the charm."""
    return os.path.join('/srv', _service())


def _config_path():
    """Return the path to the service configuration file."""
    return os.path.join(_root(), 'config.yaml')


def dashboard_path():
    """Return the path to Juju Dashboard folder."""
    return os.path.join(_root(), 'dashboard')


def _service_failed():
    """Report whether the service has failed."""
    return host.service('is-failed', _service())


def _service_enabled():
    """Report whether the service has been enabled."""
    return host.service('is-enabled', _service())


def _service_running():
    """Report whether the service is running."""
    return host.service_running(_service())


def _dashboard_resource_nonempty():
    dashboard_file = hookenv.resource_get('dashboard')
    if dashboard_file:
        return os.path.getsize(dashboard_file) != 0
    return False
