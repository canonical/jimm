JIMM Facade
===========

In addition to the facades required to emulate a juju controller, JIMM
also advertises a JIMM facade with some additional features.

Version 1
---------

Version 1 of the JIMM facade is not known to have been used by any
clients and consisted of a single procedure, UserModelStats.

### UserModelStats

The UserModelStats procedure returns model statistics for all models
accessible to the currently authenticated user.

```
UserModelStats() -> {
  "Models": {
    "fecd93ac-e082-40ce-a75b-ad5585103768": {
      "name": "test01",
      "uuid": "fecd93ac-e082-40ce-a75b-ad5585103768",
      "type": "iaas",
      "owner-tag": "user-owner@external",
      "counts": {
        "units": 10,
        "applications": 6,
        "machines": 5
      }
    },
    ...
  }
}
```

Version 2
---------

Version 2 of the JIMM facade includes an unchanged UserModelStats
procedure, and introduces two new procedures:

 - DisableControllerUUIDMasking
 - ListControllers

### DisableControllerUUIDMasking

The `DisableControllerUUIDMasking` procedure can only be used by
admin-level users. Once called any subsequent requests to a model
procedure that includes a controller UUID will use the UUID of the juju
controller hosting the model and not the the UUID of JAAS. This procedure
does not normally return any value.

### ListControllers

The `ListControllers` procedure can only be used by admin-level users. The
procedure returns the list of juju controllers that are hosting models
for the JAAS system.

```
ListControllers() -> {
  "controllers": [
    {
      "path": "controller-admin/aws-us-east-1-001",
      "public": true,
      "uuid": "e95e254b-2502-4907-83a5-2897503ce3cf",
      "version": "2.6.10"
    },
    ...
  ]
}
```
