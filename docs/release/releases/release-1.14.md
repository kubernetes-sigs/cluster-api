# Cluster API v1.14

## Timeline

The following table shows the preliminary dates for the `v1.14` release cycle.

| **What**                                             | **Who**      | **When**                    | **Week** |
|------------------------------------------------------|--------------|-----------------------------|----------|
| Start of Release Cycle                               | Release Lead | Monday 27th April 2026      | week 1   |
| Schedule finalized                                   | Release Lead | Friday 1st May 2026         | week 1   |
| Team finalized                                       | Release Lead | Friday 1st May 2026         | week 1   |
| *v1.12.x & v1.13.x released*                         | Release Lead | Tuesday 19th May 2026       | week 4   |
| KubeCon India (18–19 June 2026) idle week            | ---          | ---                         | week 8   |
| *v1.12.x & v1.13.x released*                         | Release Lead | Tuesday 23rd June 2026      | week 9   |
| v1.14.0-beta.0 released                              | Release Lead | Tuesday 14th July 2026      | week 12  |
| *v1.12.x & v1.13.x released*                         | Release Lead | Tuesday 14th July 2026      | week 12  |
| Communicate beta to providers                        | Comms Lead   | Tuesday 14th July 2026      | week 12  |
| Communicate upcoming code freeze to the community    | Comms Lead   | Tuesday 14th July 2026      | week 12  |
| v1.14.0-beta.x released                              | Release Lead | Tuesday 21st July 2026      | week 13  |
| release-1.14 branch created (**Begin Code Freeze**)  | Release Lead | Tuesday 28th July 2026      | week 14  |
| v1.14.0-rc.0 released                                | Release Lead | Tuesday 28th July 2026      | week 14  |
| release-1.14 jobs created                            | CI Lead      | Tuesday 28th July 2026      | week 14  |
| v1.14.0-rc.x released                                | Release Lead | Tuesday 4th August 2026     | week 15  |
| **v1.14.0 released**                                 | Release Lead | Tuesday 11th August 2026    | week 16  |
| *v1.12.x & v1.13.x released*                         | Release Lead | Tuesday 11th August 2026    | week 16  |
| Organize release retrospective                       | Release Lead | TBC                         | week 16  |
| *v1.14.1 released (tentative)*                       | Release Lead | Tuesday 18th August 2026    | week 17  |

After CAPI v1.14.0, the .1 release will follow up to add support for Kubernetes 1.37.0 when it becomes available. After .1, we expect to do monthly patch releases (more details will be provided in the v1.15 release schedule).

## Release team

| **Role**                                  | **Lead** (**GitHub / Slack ID**)                                                                 | **Team member(s) (GitHub / Slack ID)** |
|-------------------------------------------|--------------------------------------------------------------------------------------------------|----------------------------------------|
| Release Lead                              | Arshadd Banoo ([@arshadd-b](https://github.com/arshadd-b) / `@arshadda`)                 | Aman Shrivastava ([@aman4433](https://github.com/aman4433) / `@AmanShrivastava`) <br> Prajyot Parab ([@prajyot-parab](https://github.com/prajyot-parab) / `@Prajyot Parab`) <br> |
| Communications/Docs/Release Notes Manager | Ira Pandey ([@irapandey](https://github.com/irapandey) / `@irapandey`)               | Agustina Barbetta ([@aibarbetta](https://github.com/aibarbetta) / `@aibarbetta`) <br> Chandan Kumar ([@chandankumar4](https://github.com/chandankumar4) / `@chandankr`) <br> Vishal Anarase ([@vishalanarase](https://github.com/vishalanarase) / `@Vishal Anarase`) <br> Omar Nasser ([@onasser1](https://github.com/onasser1) / `@Omar Nasser`) <br> |
| CI Signal/Bug Triage/Automation Manager   | Peppi-Lotta ([@Peppi-Lotta](https://github.com/Peppi-Lotta) / `@Peppi-Lotta`)                   | Daniel Giszpenc ([@Daniel-Giszpenc](https://github.com/Daniel-Giszpenc) / `@Daniel`) <br> Manali Latkar ([@manalilatkar](https://github.com/manalilatkar) / `@Manali Latkar`) <br> Sathvik S ([@sats-23](https://github.com/sats-23) / `@sats`) <br> Shwetha Poojary ([@shwetha-s-poojary](https://github.com/shwetha-s-poojary) / `@Shwetha`) <br> Yuedong Wu ([@lunarwhite](https://github.com/lunarwhite) / `@lunarwhite`) <br> |
| Emeritus Advisor                          | Matt Boersma ([@mboersma](https://github.com/mboersma) / `@mboersma`)                             |                                        |