from charmhelpers.core import hookenv


def charm_status(msg):
    if not msg:
        return 'blocked', 'unknown error'
    if msg.find('mongo-addr') == -1:
        return 'blocked', msg
    # there is no mongodb connection yet, check if the relation exists.
    if hookenv.relations_of_type('db'):
        return 'waiting', 'wating for mongodb'
    return 'blocked', msg
