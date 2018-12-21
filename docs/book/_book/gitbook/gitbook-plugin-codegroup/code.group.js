require([
    'jquery',
    'gitbook',
], function ($, gitbook) {
    var self = self || {};
    var active = 'gbcg-active';
    var storageKey = 'codegroup';

    var getCodeGroupStore = function () {
        var codeGroupStore = gitbook.storage.get(storageKey);
        return codeGroupStore || {rememberTabs: {}};
    };

    self.showtab = function showtab(event) {
        event.preventDefault(); event.stopPropagation();
        var $selector = $(this);
        var $codeGroup = $selector.closest('.gbcg-codegroup');
        var $container = $codeGroup.find('#gbcg-tab-container');
        $codeGroup.find('.gbcg-selector').removeClass(active);
        var tabId = $selector.attr('data-tab');
        var selectorId = $selector.attr('id');
        var $tab = $('#' + tabId);
        $container.html($tab.html());
        $selector.addClass(active);

        var codeGroupStore = getCodeGroupStore();
        var codeGroupId = $codeGroup.attr('id');

        if ($codeGroup.attr('data-remember-tabs') === 'true') {   
            codeGroupStore.rememberTabs[codeGroupId] = selectorId;
        } else {
            delete codeGroupStore.rememberTabs[codeGroupId];
        }
        gitbook.storage.set(storageKey, codeGroupStore);
    };

    var setup = function () {
        var $selectors = $('.gbcg-selector');
        $selectors.unbind('click', self.showtab);
        $selectors.click(self.showtab);

        var $codeGroups = $('.gbcg-codegroup');
    
        $codeGroups.each(function () {
            var $group = $(this);
            var codeGroupStore = getCodeGroupStore();
            var id = $group.attr('id');
            if ($group.attr('data-remember-tabs') !== 'true') {
                delete codeGroupStore.rememberTabs[id];
                gitbook.storage.set(storageKey, codeGroupStore);
                return;
            }
            var selectorId = codeGroupStore.rememberTabs[id];
            if (selectorId) {
                var $selector = $group.find('#' + selectorId);
                $selector.click();
            }
        });
    };

    gitbook.events.bind('page.change', setup);
    setup();
    return self;
});
