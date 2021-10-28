# CRD Reference

<style>
  .content {
    width: 100%
  }
  .content main {
    max-width: 100%
  }
</style>
<script>

  const SEARCH_HOTKEY_KEYCODE = 83;
  // hasFocus returns true if the table search is active
  function hasFocus() {
    const tableSearchBar = document.querySelector("#content > main > p > rapi-doc");
    return (tableSearchBar === document.activeElement);
  }

  // Prevent the mdbook search event listener capturing the 's' key, so users can search for CRD properties.
  function resetKeyHandler(e) {
    if (e.altKey || e.ctrlKey || e.metaKey || e.shiftKey || e.target.type === 'textarea' || e.target.type === 'text') { return; }

    if (e.keyCode === SEARCH_HOTKEY_KEYCODE && hasFocus()) {
        e.stopPropagation();
    }
  }

  // Insert the event listener when the document is ready
  $(function() {
    document.addEventListener('keydown', function (e) { resetKeyHandler(e); }, true);
  });

</script>

<script type="module" src="https://unpkg.com/rapidoc/dist/rapidoc-min.js"></script>
<rapi-doc
  spec-url = "crds.yaml"
   allow-server-selection = 'false'
   schema-style="table"
   fill-request-fields-with-example='false'
   render-style = 'focused'
   allow-authentication = 'false'
   allow-try = 'false'
   allow-spec-file-load = 'false'
   allow-spec-url-load = 'false'
   show-components = 'true'
/>
