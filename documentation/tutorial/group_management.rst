Group and access management
================

Introduction
------------

JAAS provides group management capabilities, this allows JAAS 
administrators to add and remove users to and from groups and 
allow or disallow group access to various resources.

In this tutorial we will show you how to manage groups in JAAS and how to grant
access to those groups.

Prerequisites
-------------

For this tutorial you will need the following:

- Deployed JAAS system (JIMM and Candid) with a few valid users (see :doc:`../how-to/deploy_jimm` and :doc:`../how-to/deploy_candid`)
- At least one controller connected to JIMM  (see :doc:`../how-to/add_controller`)
- jimmctl command (either built from source or installed via a snap)

Group management
----------------

For this part of the tutorial we will assume your Candid has been configured
to contain the following users:

- alice
- adam
- eve

Next, let us create three groups for these users. Run: 

.. code:: console

    jimmctl auth group add A
    jimmctl auth group add B
    jimmctl auth group add C

which will create groups *A*, *B* and *C*.

To add users to groups, let's run:

.. code:: console

    jimmctl auth add relation user-alice member group-A
    jimmctl auth add relation user-adam member group-B
    jimmctl auth add relation user-eve member group-C

which will add *alice* to group *A*, *adam* to group *B* and *eve* to group *C*.
You will notice that we refer to user and group by their *JAAS tags* (for 
explanation see :doc:`../explanation/jaas_tags`).

Now to make things a bit more interesting we will make group *A* member of 
group *B* and group *B* member of group *C* by running: 

.. code:: console

    jimmctl auth relation add group-A#member member group-B
    jimmctl auth relation add group-B#member member group-C

Note the special *group-A#member* notation by which we refer to members of 
group *A*. In effect the first of these two commands tells JAAS that all users
that have a *member* relation to group *A* also have a *member* relation to
group *B*. And likewise the second command tells JAAS that all users that
have a *member* relation to group *B* also have a *member* relation to group *C*.

To list all groups known to JAAS run:

.. code:: console
    
    jimmctl group list

which will show us the three groups we created.

Let's assume we want to rename group *C* to *D*. To achieve this we run:

.. code:: console

    jimmctl auth group rename C D

If we run:

.. code:: console

    jimmctl group list

we will see groups *A*, *B* and *D*. 

Renaming a group **does not** affect group membership or any access rights a group
might already have in JAAS. This means that members of groups *A* and *B* are
still members of group *D*.

To remove group *D* from JAAS, we run:

.. code:: console

    jimmctl auth group remove D

And now listing groups will show only groups *A* and *B*.

Granting access to groups
-------------------------

Now that we know how to manage groups and group membership let's take a look
at how we can grant groups access to resources in JIMM. Remember that we
will refer to resources by their JAAS tags (for 
explanation see :doc:`../explanation/jaas_tags`).

For this tutorial we will assume:

- that you have followed the previous part of the tutorial and have
    - three users *alice*, *adam* and *eve*
    - two groups *A* and *B* set up during part one of this tutorial
- that you have added controller *test-ctl-1* to JIMM
- that you have added a model *test-model-1* on the same controller
- that you have deployed postgresql in this model and created and application offer names *postgresql-db*

First let us make user *eve* an administrator of controller *test-ctl-1*. Since
*eve* is not member of any group, we will add a direct relation between the 
user and the controller by running: 

.. code:: console

    jimmctl auth relation add user-eve administrator controller-test-ctl-1

Now let us make group *A* writer on the *test-model-1* model. Having write access
to a model means users are able to deploy applications in the model and
manage deployed applications. To achieve this run:

.. code:: console

    jimmctl auth relation add group-A#members writer model-test-ctl-1/test-model-1

And finally let us give members of group *B* consume permission on the created
application offer by running: 

.. code:: console

    jimmctl auth relation add group-B#members consumer applicationoffer-test-ctl-1/test-model-1.postgresql-db


Now let us check if *adam* has consume access to the application offer
by running: 

.. code:: console

    jimmctl auth relation check user-adam consumer applicationoffer-test-ctl-1/test-model-1.postgresql-db

We should get a positive answer since *adam* is member of group *B* and 
we have granted members of group *B* consume access to the application offer.

To remove group *B*'s access to the application offer we can run:

.. code:: console

    jimmctl auth relation remove user-adam consumer applicationoffer-test-ctl-1/test-model-1.postgresql-db

Running: 

.. code:: console

    jimmctl auth relation check user-adam consumer applicationoffer-test-ctl-1/test-model-1.postgresql-db
 
we will see user *adam* no longer has access to the application offer.

Conclusion 
----------

This tutorial taught you the basics of group and access management in JAAS. 

