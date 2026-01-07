(manage-offers)=
# Manage offers
> See also: {ref}`offer`


(control-user-access-to-an-offer)=
## Control user access to an offer

To grant a (collection of) user(s) access to an application offer, add a `reader`, `consumer`, or `administrator` permission between the user(s) and the offer. For example:

For example:

```text
# Let Alice consume offer myoffer:
juju add-permission user-alice@canonical.com consumer applicationoffer-mycontroller/mymodel.myoffer
```

> See more: {ref}`manage-permissions`

