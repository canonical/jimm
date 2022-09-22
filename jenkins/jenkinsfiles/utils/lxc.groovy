/* groovylint-disable CompileStatic, LineLength */
void launchContainer(String release) {
    sh """
        lxc launch --ephemeral ${release} ${env.BUILD_TAG}
    """
    s('while [ ! -f /var/lib/cloud/instance/boot-finished ]; do sleep 0.1; done')
}

void pushWorkspace() {
    sh '''
        lxc file push -r ./ $BUILD_TAG/home/ubuntu --create-dirs
    '''
    // Can't use --uid/--gid/--mode in lxc file push recursive mode
    // So we just chown it after the fact.
    s('sudo chown -R ubuntu:root ./')
}
//
void pullFileFromHome(String path) {
    cmd =  "lxc file pull ${env.BUILD_TAG}/home/ubuntu/${path}" 
    echo "${cmd}"
    sh "${cmd}"
}

void s(String command, List<String> envArgs=[]) {
            // export http_proxy=${params.http_proxy}s; \
            // export https_proxy=${params.http_proxy}; \s
            // --env HTTP_PROXY==${env.HTTP_PROXY} --env HTTPS_PROXY=${env.HTTPS_PROXY}
    String cmd = "lxc exec ${env.BUILD_TAG} --cwd /home/ubuntu/${env.JOB_BASE_NAME} --user 1000 --env HOME=/home/ubuntu"
    envArgs.each { env -> cmd <<= (' --env ' + env + ' ') }
    cmd <<= ' -- bash -c '
    cmd <<= "\'${command.trim()}\'"
    sh(script: "${cmd}")
}

void installSnap(String name, Boolean classic=false, String channel='') {
    String cmd = 'sudo snap install '
    cmd <<= name
    if (classic) {
        cmd <<= ' --classic'
    }
    if (channel != '') {
        cmd <<= " --channel=${channel}"
    }
    s(cmd)
}

void removeContainer() {
    sh '''
        lxc delete $BUILD_TAG --force
    '''
}

return this
