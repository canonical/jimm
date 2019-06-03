import os

from charmhelpers.core import hookenv
import jaascharm

if __name__ == '__main__':
    hookenv.log('install')
    jaascharm.install(binary=os.path.join('bin', 'jemd'))
