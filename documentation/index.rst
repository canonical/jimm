:relatedlinks: [Diátaxis](https://diataxis.fr/)

.. _home:

JAAS Documenation
==========================

**JAAS** provides a single location to manage your Juju infrastructure by using the 
Dashboard or using the same Juju CLI commands to create a high-level overview and 
the ability to drill-in to the details when you need it.

**JAAS** is composed of the following components:
- Candid - a macaroon-based authentication server, which is required so that all other components are able to agree who a particular user is

- JIMM - Juju Intelligent Model Manager, which acts as a single point of contact for multiple Juju controllers

- Juju controllers -  each controlling models in specific clouds or cloud regions.

- Juju dashboard - providing a clear overview of your Juju real estate with the ability to drill down into details of your deploys

**JAAS** is useful for customers that do not want to maintain their own controllers
in public clouds taking advantage of our production JAAS giving them the ability to
deploy their workloads in public clouds. JAAS is also useful for organisations 
running their own Juju infrastructure giving them a single point of contact for 
their entire real estate and, in combination with the Juju Dashboard, giving
them a clear overview of their infrastructure.

---------

In this documentation
---------------------

..  grid:: 1 1 2 2

   ..  grid-item:: :doc:`Tutorial <tutorial/index>`


   ..  grid-item:: :doc:`How-to guides <how-to/index>`


.. grid:: 1 1 2 2
   :reverse:

   .. grid-item:: :doc:`Reference <reference/index>`


   .. grid-item:: :doc:`Explanation <explanation/index>`


---------

Project and community
---------------------

Example Project is a member of the Ubuntu family. It’s an open source project that warmly welcomes community projects, contributions, suggestions, fixes and constructive feedback.

* :ref:`Code of conduct <home>`
* :ref:`Get support <home>`
* :ref:`Join our online chat <home>`
* :ref:`Contribute <home>`


.. toctree::
   :hidden:
   :maxdepth: 2

   tutorial/index
   how-to/index
   reference/index
   explanation/index
