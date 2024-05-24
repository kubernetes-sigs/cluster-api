# Cluster API v1.7

## Timeline

The following table shows the preliminary dates for the `v1.7` release cycle.

| **What**                                             | **Who**      | **When**                    | **Week** |
|------------------------------------------------------|--------------|-----------------------------|----------|
| Start of Release Cycle                               | Release Lead | Monday 11th December 2023   | week 1   |
| Schedule finalized                                   | Release Lead | Friday 15th December 2023   | week 1   |
| Team finalized                                       | Release Lead | Friday 15th December 2023   | week 1   |
| *v1.5.x & v1.6.x released*                           | Release Lead | Tuesday 16th January 2024   | week 6   |
| *v1.5.x & v1.6.x released*                           | Release Lead | Tuesday 13th February 2024  | week 10  |
| v1.7.0-beta.0 released                               | Release Lead | Tuesday 12th March 2024     | week 14  |
| Communicate beta to providers                        | Comms Manager| Tuesday 12th March 2024     | week 14  |
| *v1.5.x & v1.6.x released*                           | Release Lead | Tuesday 12th March 2024     | week 14  |
| KubeCon idle week                                    |      -       | 19th - 22nd March 2024      | week 15  |
| v1.7.0-beta.x released                               | Release Lead | Tuesday 26th March 2024     | week 16  |
| release-1.7 branch created (**Begin [Code Freeze]**) | Release Lead | Tuesday 2nd April 2024      | week 17  |
| v1.7.0-rc.0 released                                 | Release Lead | Tuesday 2nd April 2024      | week 17  |
| release-1.7 jobs created                             | CI Manager   | Tuesday 2nd April 2024      | week 17  |
| v1.7.0-rc.x released                                 | Release Lead | Tuesday 9th April 2024      | week 18  |
| **v1.7.0 released**                                  | Release Lead | Tuesday 16th April 2024     | week 19  |
| *v1.5.x & v1.6.x released*                           | Release Lead | Tuesday 16th April 2024     | week 19  |
| Organize release retrospective                       | Release Lead | TBC                         | week 19  |

After the `.0` release monthly patch release will be created.

## Release team

| **Role**                                  | **Lead** (**GitHub / Slack ID**)                                                      | **Team member(s) (GitHub / Slack ID)** |
|-------------------------------------------|-------------------------------------------------------------------------------------------|----------------------------------------|
| Release Lead                              | Stephen Cahill ([@cahillsf](https://github.com/cahillsf/) / `@Stephen Cahill`) | Nawaz Hussain Khazielakha ([@nawazkh](https://github.com/nawazkh) / `@nawazkh`) <br> Akshay Gaikwad ([@akshay196](https://github.com/akshay196) / `@Akshay Gaikwad`) <br> Mohamed chiheb ben jemaa ([@mcbenjemaa](https://github.com/mcbenjemaa) / `@mcbenjemaa`) <br> Subhasmita Swain ([@SubhasmitaSw](https://github.com/SubhasmitaSw) / `@subhasmita`) <br> Vibhor Chinda ([@VibhorChinda](https://github.com/VibhorChinda) / `@Vibhor Chinda`)                                      |
| Communications/Docs/Release Notes Manager | Willie Yao ([@willie-yao](https://github.com/willie-yao) / `@willie`) | Chandan Kumar ([@chandankumar4](https://github.com/chandankumar4) / `@Chandan Kumar`) <br> Dhairya Arora ([@Dhairya-Arora01](https://github.com/Dhairya-Arora01) / `@Dhairya Arora`) <br> Suriyan S ([@ssuriyan7](https://github.com/ssuriyan7) / `@Suriyan S`) <br> Yash Raj ([@yrs147](https://github.com/yrs147) / `@Yash Raj`) <br> Typeid ([@typeid](https://github.com/typeid) / `@typeid`)                                 |
| CI Signal/Bug Triage/Automation Manager   | Muhammad Adil Ghaffar ([@adilGhaffarDev](https://github.com/adilGhaffarDev) / `@adil`) | Sunnat Samadov ([@Sunnatillo](https://github.com/Sunnatillo) / `@Sunnat`) <br> Aditya Bhatia ([@adityabhatia](https://github.com/adityabhatia) / `@adityabhatia`) <br> Mansi Kulkarni ([@mansikulkarni96](https://github.com/mansikulkarni96) / `@Mansi Kulkarni`) <br> Mike Fedosin ([@Fedosin](https://github.com/Fedosin) / `@Mike Fedosin`) <br> Troy Connor ([@troy0820](https://github.com/troy0820) / `@troy0820`)                                    |