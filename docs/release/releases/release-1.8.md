# Cluster API v1.8

## Timeline

The following table shows the preliminary dates for the `v1.8` release cycle.

| **What**                                             | **Who**      | **When**                  | **Week** |
|------------------------------------------------------|--------------|---------------------------|----------|
| Start of Release Cycle                               | Release Lead | Monday 22nd April 2024    | week 1   |
| *v1.7.x released* <sup>[1]</sup>                     | Release Lead | Tuesday 23rd April 2024   | week 1   |
| Schedule finalized                                   | Release Lead | Friday 26th April 2024    | week 1   |
| Team finalized                                       | Release Lead | Friday 26th April 2024    | week 1   |
| *v1.6.x & v1.7.x released*                           | Release Lead | Tuesday 14th May 2024     | week 4   |
| *v1.6.x & v1.7.x released*                           | Release Lead | Tuesday 11th June 2024    | week 8   |
| *v1.6.x & v1.7.x released*                           | Release Lead | Tuesday 9th July 2024     | week 12  |
| v1.8.0-beta.0 released                               | Release Lead | Tuesday 16th July 2024    | week 13  |
| Communicate beta to providers                        | Release Lead | Tuesday 16th July 2024    | week 13  |
| v1.8.0-beta.x released                               | Release Lead | Tuesday 23rd July 2024    | week 14  |
| release-1.8 branch created (**Begin [Code Freeze]**) | Release Lead | Tuesday 30th July 2024    | week 15  |
| v1.8.0-rc.0 released                                 | Release Lead | Tuesday 30th July 2024    | week 15  |
| release-1.8 jobs created                             | Release Lead | Tuesday 30th July 2024    | week 15  |
| v1.8.0-rc.x released                                 | Release Lead | Tuesday 6th August 2024   | week 16  |
| **v1.8.0 released**                                  | Release Lead | Monday 12th August 2024   | week 17  |
| *v1.6.x & v1.7.x released*                           | Release Lead | Monday 12th August 2024   | week 17  |
| *v1.8.1 released (tentative)*                        | Release Lead | Thursday 15th August 2024 | week 17  |
| Organize release retrospective                       | Release Lead | TBC                       | week 17  |

After the `.0` the .1 release will be created to ensure faster Kubernetes support after K8s 1.31.0 will be available.
After the `.1` we expect to release monthly patch release (more details will be provided in the 1.9 release schedule).

Note: This release cycle there are some additional constraints for the .1 release due to planned test infra activities;
as a consequence .1 timeline has been compressed (from 1 week / 10 days delay of the last release cycle down to 2/3 days)
and maintainers might change plans also last second to adapt to latest info about infrastructure availability.

## Release team

| **Role**                                  | **Lead** (**GitHub / Slack ID**)                                                      | **Team member(s) (GitHub / Slack ID)** |
|-------------------------------------------|-------------------------------------------------------------------------------------------|----------------------------------------|
| Release Lead                              | Muhammad Adil Ghaffar ([@adilGhaffarDev](https://github.com/adilGhaffarDev/) / `@adil`) | Dan Buch ([@meatballhat](https://github.com/meatballhat) / `@meatballhat`) <br> Nivedita Prasad ([@Nivedita-coder](https://github.com/Nivedita-coder) / `@Nivedita Prasad`) <br> Chirayu Kapoor ([@chiukapoor](https://github.com/chiukapoor) / `@Chirayu Kapoor`) <br> David Hwang ([@dhij](https://github.com/dhij) / `@David Hwang`)|
| Communications/Docs/Release Notes Manager | Chandan Kumar ([@chandankumar4](https://github.com/chandankumar4) / `@Chandan Kumar`) | Vishal Anarase ([@vishalanarase](https://github.com/vishalanarase) / `@Vishal Anarase`)  <br> Shipra Jain ([@shipra101](https://github.com/shipra101) / `@Shipra Jain`)<br> Rajan Kumar Jha ([@rajankumary2k](https://github.com/rajankumary2k) / `@Rajan Jha`)<br> Kunju Perath ([@kperath](https://github.com/kperath) / `@Kunju Perath`)  |
| CI Signal/Bug Triage/Automation Manager   | Sunnat Samadov ([@Sunnatillo](https://github.com/Sunnatillo) / `@Sunnat`) | Willie Yao ([@willie-yao](https://github.com/willie-yao) / `@willie`) <br> Troy Connor ([@troy0820](https://github.com/troy0820) / `@troy0820`)  <br> Jayesh Srivastava ([@jayesh-srivastava](https://github.com/jayesh-srivastava) / `@Jayesh`)  <br> Amit Kumar ([@hackeramitkumar](https://github.com/hackeramitkumar) / `@Amit Kumar`)  <br> Moshiur Rahman ([@smoshiur1237](https://github.com/smoshiur1237) / `@Moshiur Rahman`)  <br> Pravar Agrawal ([@pravarag](https://github.com/pravarag) / `@pravarag`)|
