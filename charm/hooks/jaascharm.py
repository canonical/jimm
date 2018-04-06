import grp
import json
import os
import pwd
import subprocess
import tarfile
import urllib.request

from charmhelpers.contrib.charmsupport.nrpe import NRPE
from charmhelpers.core import (
    hookenv,
    host,
    templating,
)
import yaml


def install(service=None, resource=None, root=None, binary=None,
            owner='root', group='root'):
    """Install the specified service service from the specified resource
       in the specified root path. A systemd unit file is also created for
       the service.

       Parameters:
       service  - The name of the service that is being installed. Defaults
                  to the name of the charm (from metadata.yaml).
       resource - The name of the resource which will be unpacked for the
                  installation. Default to be the value of {{service}}.
       root     - The root directory into which the resource will be unpacked.
                  Default is /srv/{{service}}.
       binary   - Path of the binary file that runs the service within the
                  top-level directory of the resource. Default is
                  bin/{{service}}.
       owner    - The user who should own the installed files. Default root.
       group    - The group that should own the installed files. Default root.
    """

    service = _service(service)
    root = _root(root, service)
    if not resource:
        resource = service
    if not binary:
        binary = os.path.join('bin', service)

    hookenv.status_set('maintenance', 'creating users')
    host.adduser(service)

    hookenv.status_set('maintenance', 'creating destination directories')
    host.mkdir(root, owner=owner, group=group, perms=0o755)
    host.mkdir(os.path.join(root, 'etc'), owner=owner, group=group,
               perms=0o755)

    hookenv.status_set('maintenance', 'getting {} resource'.format(resource))
    src_path = hookenv.resource_get(resource)
    if not src_path:
        hookenv.status_set('blocked',
                           'waiting for {} resource'.format(resource))
        return False

    hookenv.status_set('maintenance', 'installing {}'.format(service))
    with tarfile.open(src_path) as tf:
        tf.extractall(root)
        names = tf.getnames()
        dst_path = os.path.join(root, names[0])
        uid = pwd.getpwnam(owner).pw_uid
        gid = grp.getgrnam(group).gr_gid
        for name in names:
            os.chown(os.path.join(root, name), uid, gid)
    hookenv.status_set('maintenance', 'writing {}.service'.format(service))
    path = os.path.join(root, 'systemd', '{}.service'.format(service))
    context = {
        'dst_path': dst_path,
        'bin_path': os.path.join(dst_path, binary),
        'config_path': _config_path(service, root)}
    templating.render(
        '{}.service'.format(service),
        path,
        context,
        owner=owner,
        group=group)
    if not _service_enabled(service):
        host.service('enable', path)
    else:
        subprocess.check_call(('systemctl', 'daemon-reload'))
    return True


def stop(service=None):
    """Stop the specified service."""
    host.service_stop(_service(service))


def update_config(config, service=None, root=None, owner='root', group='root'):
    """Update the yaml configuration file for the specified service with the
       configuration keys specified in config. If a config key has a value of
       None then that value will be removed from the configuration file, if
       present. The return value indicates whether the config file was changed
       or not."""
    path = _config_path(service, root)
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

    host.write_file(
        path,
        yaml.safe_dump(data),
        owner=owner,
        group=group,
        perms=0o644)
    return changed


def update_config_and_restart(config, service=None, root=None, owner='root',
                              group='root'):
    """Update the YAML configuration file for service with config and restart
       service. The service will only be restarted if the configuration is
       changed or the service is already not yet running."""
    service = _service(service)
    changed = update_config(config, service, root, owner, group)
    if not _service_enabled(service):
        return
    if host.service_running(service):
        if not changed:
            return
        hookenv.status_set('maintenance',
                           'restarting {} service'.format(service))
        host.service_restart(service)
    else:
        hookenv.status_set('maintenance',
                           'starting {} service'.format(service))
        host.service_start(service)


def port_changed(port, old_port):
    """Change the service port from old_port to port."""
    if port == old_port:
        return
    hookenv.log('port changed from {} to {}'.format(old_port, port))
    hookenv.open_port(port)
    if old_port is not None:
        hookenv.close_port(old_port)
    for rel in hookenv.relation_ids('website'):
        hookenv.relation_set(rel, port=port)
    if hookenv.relation_ids('nrpe'):
        update_nrpe_config()


def update_nrpe_config(service=None, nrpe=NRPE(), config=hookenv.config()):
    """Update the NRPE configuration for the given service."""
    hookenv.log("updating NRPE checks")
    service = _service(service)

    nrpe.add_check(
        shortname=service,
        description="Check {} running".format(service),
        check_cmd="check_http -w 2 -c 10 -I {} -p {} -u /debug/info".format(
            hookenv.unit_private_ip(),
            config['port'])
    )
    nrpe.write()


def update_status(service=None, failed_status=None):
    """Update the status message for the specified service.
       If failed_status is specified it will be called when the service is
       determined to have failed. failed_status should return a tuple
       containing the status set and the message."""
    service = _service(service)
    if host.service_running(service):
        _set_active_status()
    if _service_failed(service):
        failed_status = failed_status or _default_failed_status
        status, msg = failed_status(_failed_msg(service))
        hookenv.status_set(status, msg)


def _set_active_status():
    """Set the status for a service that has been detected as active."""
    url = "http://localhost:{}/debug/info".format(hookenv.config('port'))
    try:
        with urllib.request.urlopen(url) as resp:
            buf = resp.read().decode('utf-8')
            data = json.loads(buf)
    except Exception as e:
        hookenv.log('cannot get version: {}'.format(e))
        return
    hookenv.status_set('active', '')
    hookenv.application_version_set(data.get('Version', ''))


def _failed_msg(service):
    """Determine the reason the given service is failed by checking the
       service logs."""
    cmd = ('journalctl', '-u', service, '-o', 'cat', '-n', '20')
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


def _service(service):
    """Determine the name of the service to operate on, either using
       the given service name or, if that is not set, using the charm
       name from metadata.yaml."""
    if service:
        return service
    return hookenv.metadata()['name']


def _root(root, service):
    """Determine the root directory for installations for the charm. If
       valid the value of root will be returned otherwise the value will
       be calculated as /srv/{{service}}."""
    if root:
        return root
    return os.path.join('/srv', service)


def _config_path(service=None, root=None):
    """Determine the configuration file path.

       Parameters:
       service  - The name of the service that is being installed. Defaults
                  to the name of the charm (from metadata.yaml).
       root     - The root directory into which the resource will be unpacked.
                  Default is /srv/{{service}}.
    """
    service = _service(service)
    return os.path.join(_root(root, service), 'etc', '{}.yaml'.format(service))


def _service_failed(service):
    """Returns True if the given service is reported as failed by systemd."""
    return host.service('is-failed', service)


def _service_enabled(service):
    """Returns True if the given service is reported as enabled by systemd."""
    return host.service('is-enabled', service)
