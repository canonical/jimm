---
myst:
  html_meta:
    description: "Learn how to manage application offers in JAAS and control user access with permissions."
---

(manage-offers)=
# Manage offers
> See first: {ref}`offer`

(control-user-access-to-an-offer)=
## Control user access to an offer

To grant a (collection of) user(s) access to an application offer, add a `reader`, `consumer`, or `administrator` permission between the user(s) and the offer.

> See more: {ref}`add-a-permission`

(remove-an-offer)=
## Remove an offer

To remove an offer in a model on a controller managed through JAAS, on your JAAS controller run:

```
juju remove-offer <offer-url>
```

```{note}
If you destroy an offer on a controller managed by JAAS directly against the backing Juju controller, you will still see it in JAAS. This happens because JIMM's database has become out-of-sync with the Juju controller. To reconcile the state, delete the offer against JAAS too.
```
