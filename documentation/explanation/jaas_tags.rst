JAAS tags
=========

Introduction
------------

When dealing with Relation Based Access Control (**ReBAC**) in JAAS we
use **JAAS tags** when referring to resources.

Each **JAAS tag** uniquely identifies a resource.

Users 
-----

A user tag has the following format:

.. code:: console 

    user-<username>

where *username* uniquely identifies a user including the domain specified
by Candid.

Group
-----

A group tag has the following format:

.. code:: console

    group-<group name>
    group-<group id>

where *group id* represents the internal ID of the group. Most commonly we
refer to groups by their group name.

Controller
----------

A controller tag has the following format:

.. code:: console

    controller-<controller name>

Cloud
----------

A cloud tag has the following format:

.. code:: console

    cloud-<cloud name>

Model
-----

A model tag has the following format:

.. code:: console

    model-<controller name>/<model name>

where *controller name* specifies name of the controller on which the model
is running and *model name* specifies the name of the model.

Application offer
-----------------

An application offer tag has the following format:

.. code:: console

    applicationoffer-<controller name>/<model name>.<offer name>

where *controller name* specifies name of the controller on which the model
is running, *model name* specifies name of the model in which the application
offer was created and *offer name* specifies the name of the application offer.