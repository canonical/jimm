(offer)=
# Offer
> See first: {external+juju:ref}`Juju | Offer <offer>`
>
> See also: {ref}`manage-offers`

(offer-tag)=
## Offer tag

An application offer tag has the following format:

```text
applicationoffer-<controller name>/<model name>.<offer name>
```

where `<controller name>` specifies name of the controller on which the model
is running, `<model name>` specifies name of the model in which the application
offer was created and `<offer name>` specifies the name of the application offer.

(offer-permission)=
## Offer permission

An offer permission describes what an entity can do with an offer.

(list-of-offer-permissions)=
### List of offer permissions

(offer-permission-administrator)=
#### `administrator`

Abilities: Can do anything that it is possible to do at the level of an offer.

(offer-permission-consumer)=
#### `consumer`

Abilities: Can relate an application to the offer.

(offer-permission-reader)=
#### `reader`

Abilities: Can view offers during a search with `juju find-offers`.

