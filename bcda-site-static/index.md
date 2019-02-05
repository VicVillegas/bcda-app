---
layout: home
title:  "Beneficiary Claims Data API"
date:   2017-10-30 09:21:12 -0500
description: Shared Savings Program ACOs can use this FHIR-based API to retrieve bulk claims data related to their assigned beneficiaries. Under construction; feedback invited.
landing-page: live
gradient: "blueberry-lime-background"
subnav-link-gradient: "blueberry-lime-link"
sections:
  - Overview
  - About the Data
ctas:
  -
    title: Visit the BCDA Google Group
    link: https://groups.google.com/forum/#!forum/bc-api

---


## Overview

  * The Beneficiary Claims Data API (BCDA) will enable Accountable Care Organizations (ACOs) participating in the Shared Savings Program to retrieve Medicare Part A, Part B, and Part D claims data for their assigned or assignable beneficiaries. This includes Medicare claims data for instances in which beneficiaries receive care outside of the ACO, allowing a full picture of patient care. When it is in production, the API will provide similar data to Claim and Claim Line Feed (CCLF) files, currently provided monthly to Shared Savings Program ACOs by CMS.
  * As of January 2019, the Beneficiary Claims Data API is a work in progress.

    * We’re currently providing synthetic data that resembles the kinds of data Shared Savings Program ACOs will receive by connecting with this endpoint, so that they can try out different ways of interacting with the API before receiving live data.

    * We’re providing this test experience and documentation early so we can learn from Shared Savings Program ACOs and their vendor partners who need to access this information, about what works best for them. Through conversations and test drives, we strive to learn what ACOs need from bulk beneficiary claims data, and create a process that meets their needs.

    * Developers, analysts, and administrators at Shared Savings Program ACOs are invited to participate in this pilot process by joining our Google Group.

  * What has been learned from ACOs in the pilot?

    Developers, analysts, and administrators at ACOs have been instrumental in shaping CMS’ approach to this API. With their feedback, the team is implementing the following elements:

    * Providing clear, human-readable narrative documentation to aid all users’ use of the API and the data that is shared
    * Using resilient NDJSON ([New Line Delimited JSON](http://ndjson.org)) rather than fixed-width files in response to requests for delimited information
    * Providing bulk beneficiary claims data through an automated retrieval process that requires minimal hands-on intervention to receive
    * Formatting data in accordance with robust Fast Health Interoperability Resource ([FHIR](https://www.hl7.org/fhir/overview.html)) specifications

    BCDA will continue to take an iterative approach to testing and learning from its users.

  * {:.bcda_callout .no_list} How can I get access to BCDA? ![Google Groups Logo](assets/img/google_logo.png){:#google_logo}

     * {:.no_list} While the Beneficiary Claims Data API (BCDA) is still in development, we'll share updates about how to get access via the [Google Group](https://groups.google.com/forum/#!forum/bc-api).

     * {:.no_list} The BCDA Google Group is the best place to get your questions answered by the BCDA team. In this community you can sign up for feedback session opportunities, get answers to your questions, share your feedback and ideas, and get updates on the project.

     * {:.button .no_list}[Join the BCDA Google Group](https://groups.google.com/forum/#!forum/bc-api)

## About the Data

   * If you're used to working with CCLF files, you'll want to know more about the data we're using and how to work with it.

     For data formatting, BCDA follows the workflow outlined by the [FHIR Bulk Data Export Implementation Guide](https://github.com/smart-on-fhir/fhir-bulk-data-docs/blob/master/export.md), using the [HL7 FHIR Standard](https://www.hl7.org/fhir/).

     Claims data is provided as FHIR resources in [NDJSON](http://ndjson.org/) format.

   * What is FHIR ([Fast Healthcare Interoperability Resources](https://www.hl7.org/fhir/))?   

     * FHIR is a specification for how servers that provide healthcare records should be set up.

     * FHIR provides a framework for the exchange of healthcare-related data, allowing any system to access and consume this data to solve clinical and administrative problems around healthcare-related data.
     * BCDA will be using the following endpoints from the FHIR spec:
        * patient endpoint
        * explanation of benefits endpoint
        * coverage endpoint

   * How is BCDA different from Blue Button 2.0? 

     * Blue Button 2.0 provides FHIR-formatted data for one individual Medicare beneficiary at a time, to registered applications with beneficiary authorization. See [https://bluebutton.cms.gov/](https://bluebutton.cms.gov/).
     * BCDA provides FHIR-formatted bulk data files to an ACO for all of the beneficiaries eligible to a given Shared Savings Program ACO. BCDA does not require individual beneficiary authorization. 