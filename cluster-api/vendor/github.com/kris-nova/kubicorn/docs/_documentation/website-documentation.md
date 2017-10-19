---
layout: documentation
title: Website Documentation
date: 2017-10-18
doctype: general
---

# kubicorn.io

Here is the content of the official `kubicorn` website: [kubicorn.io](http://kubicorn.io).

All documentation for the project is hosted in the [docs section](http://kubicorn.io/documentation/readme.html) of [kubicorn.io](http://kubicorn.io).

The website runs on Jekyll, straight from GitHub Pages. It consists of templates and markdown files that are automatically built into HTML pages whenever any of the content changes. 

The most common edits are changing the home.html file (our index page), and adding or editing files in the documentation/ folder.

# Website structure

```
kubicorn/docs/
│
├── assets/
│ → This folder basically contains the favicons. We should probably move this
│   to img/ sometime.
│   
├── docs_old/
│ → These are the old docs from before kubicorn had a website.
│
├── _documentation/
│ → All docs to be displayed on in the Documentation section of the website
│   should go here. They should be markdown, and include the YAML header as
│    shown in the *Adding a new page* section below.
│
├── _friends/
│ → Files in this folder feed the *Friends of kubicorn* section of the website.
│   You should follow the formatting as per the files already present.
│
├── _includes/
│ → This holds the includes for every page of the website: footer, header, and
│   head sections.
│
├── _layouts/
│   │ → This holds the different templates that make up the website. In
│   │   practice we're only using two:
│   │
│   ├── documentation.html
│   │ → This is the template used to generate all files in the documentation
│   │   section.
│   │
│   └── home.html
│     → This is the website's index page. Most of the text in the index page
│       is hard-coded here. Exceptions are the _friends/ content and the
│       _documentation/ list, which are generated dynamically.
│
├── img/
│ → All images go here.
│
├── _posts/ & _sass/
│ → These are defaults from the initial Jekyll installation, and should be
│   cleaned up at some point.
│
└── _site/
  → This folder contains the auto-generated files that the website serves. They
    are built automatically whenever anything else on the folders above change,
    and should not be edited manually. Any changes to these files will be
    discarded. 

```

## Adding a new page

To create a new page on the website, create a new `.md` markdown file in `/docs/_documentation`.

All new docs must contain the header:

```
---
layout: documentation
title: [The title of your document]
date: YYYY-MM-DD
doctype: [general/aws/azure/do/google]
---
```

Where `doctype` is the larger category for the documentation (valid categories are `general`, `aws`, `azure`, `do`, and `google`). All docs should be written in valid `.md` markdown.

Do not include a title top level header on the document, e.g. `# Title`. The title is pulled from `title: value` on the section above. You can start your file with a level two header, e.g. `## Second level header`, or go straight to normal text.

## Editing existing documentation

Simply update the associated `.md` markdown file. All documentation should be in complete sentences.

# Testing changes

If you have Jekyll stack installed you can run the website locally to test changes. To install Jekyll, follow the [Setting up your GitHub Pages site locally with Jekyll](https://help.github.com/articles/setting-up-your-github-pages-site-locally-with-jekyll/) tutorial.  
Start the website by running the following command from the `/docs` directory:
```
bundle exec jekyll serve
```
The website will be available at `localhost:4000`.

