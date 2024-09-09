## Version History

JIMM v0 and v1 follow a different versioning strategy than future releases. JIMM v0 was the initial
release and used MongoDB to store state. JIMM v1 was an upgrade that switched to using PostgreSQL 
for storing state but still retained similar functionality to v0.  
These versions worked with Juju v2 and v3.

Subsequently JIMM introduced a large shift in how the service worked:
- JIMM now acts as a proxy between all client and Juju controller interactions. Previously 
users were redirected to a Juju controller.
- Juju controllers support JWT login where secure tokens are issued by JIMM.
- JIMM acts as an authorisation gateway creating trusted short-lived JWT tokens to authorize 
user actions against Juju controllers.

The above work encompassed a breaking change and required changes in Juju (requiring a 
Juju controller of at least version 3.3). 

Further, to better align the two projects, JIMM's versioning now aligns with Juju.

As a result of this, there is no JIMM v2 and instead from JIMM v3, the versioning strategy 
we follow is to match JIMM's major version to the corresponding Juju major version we support.
