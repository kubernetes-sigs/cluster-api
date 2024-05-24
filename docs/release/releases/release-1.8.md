# Cluster API v1.8

## Timeline

The following table shows the preliminary dates for the `v1.8` release cycle.

| **What**                                             | **Who**      | **When**                    | **Week** |
|------------------------------------------------------|--------------|-----------------------------|----------|
| Start of Release Cycle                               | Release Lead | Monday 22nd April 2024      | week 1   |
| *v1.7.x released* <sup>[1]</sup>                     | Release Lead | Tuesday 23rd April 2024     | week 1   |
| Schedule finalized                                   | Release Lead | Friday 26th April 2024      | week 1   |
| Team finalized                                       | Release Lead | Friday 26th April 2024      | week 1   |
| *v1.6.x & v1.7.x released*                           | Release Lead | Tuesday 14th May 2024       | week 4   |
| *v1.6.x & v1.7.x released*                           | Release Lead | Tuesday 11th June 2024      | week 8   |
| *v1.6.x & v1.7.x released*                           | Release Lead | Tuesday 9th July 2024       | week 12  |
| v1.8.0-beta.0 released                               | Release Lead | Tuesday 16th July 2024      | week 13  |
| Communicate beta to providers                        | Release Lead | Tuesday 16th July 2024      | week 13  |
| v1.8.0-beta.x released                               | Release Lead | Tuesday 23rd July 2024      | week 14  |
| release-1.8 branch created (**Begin [Code Freeze]**) | Release Lead | Tuesday 30th July 2024      | week 15  |
| v1.8.0-rc.0 released                                 | Release Lead | Tuesday 30th July 2024      | week 15  |
| release-1.8 jobs created                             | Release Lead | Tuesday 30th July 2024      | week 15  |
| v1.8.0-rc.x released                                 | Release Lead | Tuesday 6th August 2024     | week 16  |
| **v1.8.0 released**                                  | Release Lead | Tuesday 13th August 2024    | week 17  |
| *v1.6.x & v1.7.x released*                           | Release Lead | Tuesday 13th August 2024    | week 17  |
| Organize release retrospective                       | Release Lead | TBC                         | week 17  |

After the `.0` release monthly patch release will be created.

<sup>[1]</sup> Pending Kubernetes `v1.30` release [scheduled for April 17th](https://github.com/kubernetes/sig-release/tree/master/releases/release-1.30).  This will be the first release cycle implementing faster Kubernetes support as introduced in [this PR](https://github.com/kubernetes-sigs/cluster-api/pull/9971/files#diff-2bf0df29da8afa5540cf65166b0b876393482bedef71d023d87835bb1b3ecbcb).

## Release team

| **Role**                                  | **Lead** (**GitHub / Slack ID**)                                                      | **Team member(s) (GitHub / Slack ID)** |
|-------------------------------------------|-------------------------------------------------------------------------------------------|----------------------------------------|
| Release Lead                              | Muhammad Adil Ghaffar ([@adilGhaffarDev](https://github.com/adilGhaffarDev/) / `@adil`) | Dan Buch (@meatballhat](https://github.com/meatballhat) / `@meatballhat`) <br> Nivedita Prasad ([@Nivedita-coder](https://github.com/Nivedita-coder) / `@Nivedita Prasad`) <br> Chirayu Kapoor ([@chiukapoor](https://github.com/chiukapoor) / `@Chirayu Kapoor`) <br> David Hwang ([@dhij](https://github.com/dhij) / `@David Hwang`)|
| Communications/Docs/Release Notes Manager | Chandan Kumar ([@chandankumar4](https://github.com/chandankumar4) / `@Chandan Kumar`) | Vishal Anarase ([@vishalanarase](https://github.com/vishalanarase) / `@Vishal Anarase`)  <br> Shipra Jain ([@shipra101](https://github.com/shipra101) / `@Shipra Jain`)<br> Rajan Kumar Jha ([@rajankumary2k](https://github.com/rajankumary2k) / `@Rajan Jha`)<br> Kunju Perath ([@kperath](https://github.com/kperath) / `@Kunju Perath`)  |
| CI Signal/Bug Triage/Automation Manager   | Sunnat Samadov ([@Sunnatillo](https://github.com/Sunnatillo) / `@Sunnat`) | Willie Yao ([@willie-yao](https://github.com/willie-yao) / `@willie`) <br> Troy Connor ([@troy0820](https://github.com/troy0820) / `@troy0820`)  <br> Jayesh Srivastava ([@jayesh-srivastava](https://github.com/jayesh-srivastava) / `@Jayesh`)  <br> Amit Kumar ([@hackeramitkumar](https://github.com/hackeramitkumar) / `@Amit Kumar`)  <br> Moshiur Rahman ([@smoshiur1237](https://github.com/smoshiur1237) / `@Moshiur Rahman`)  <br> Pravar Agrawal ([@pravarag](https://github.com/pravarag) / `@pravarag`)|