# kubicorn.io

Here is the content of the official `kubicorn` website: [kubicorn.io](http://kubicorn.io).

All documentation for the project is hosted in the [docs section](http://kubicorn.io/documentation/readme.html) of [kubicorn.io](http://kubicorn.io).

# Adding a new page

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

# Editing existing documentation

Simply update the associated `.md` markdown file. All documentation should be in complete sentences.

# Testing changes

If you have Jekyll stack installed you can run the website locally to test changes. To install Jekyll, follow the [Setting up your GitHub Pages site locally with Jekyll](https://help.github.com/articles/setting-up-your-github-pages-site-locally-with-jekyll/) tutorial.  
Start the website by running the following command from the `/docs` directory:
```
bundle exec jekyll serve
```
The website will be available at `localhost:4000`.

