(function () {
    'use strict';

    let currentFilter = null;
    let filterPopup = null;

    function createFilterPopup() {
        if (filterPopup) return filterPopup;

        filterPopup = document.createElement('div');
        filterPopup.className = 'tech-filter-popup';
        filterPopup.innerHTML = `
            <span class="filter-icon">🔧</span>
            <span class="filter-text">Filtering:</span>
            <strong class="filter-tech"></strong>
            <button class="clear-filter-btn" type="button">Clear ✕</button>
        `;

        filterPopup.querySelector('.clear-filter-btn').addEventListener('click', (e) => {
            e.preventDefault();
            e.stopPropagation();
            clearTechFilter();
        });

        document.body.appendChild(filterPopup);
        return filterPopup;
    }

    function showFilterPopup(name) {
        const p = createFilterPopup();
        p.querySelector('.filter-tech').textContent = name;
        p.classList.add('show');
    }

    function hideFilterPopup() {
        if (filterPopup) filterPopup.classList.remove('show');
    }

    function clearTechFilter() {
        const projectsSection = document.querySelector('.projects-section');
        if (!projectsSection) return;

        projectsSection.querySelectorAll('.project-card').forEach(card => {
            card.style.opacity = '1';
            card.style.transform = '';
            card.style.filter = '';
        });

        document.querySelectorAll('.tech-item.filtered').forEach(x => x.classList.remove('filtered'));
        currentFilter = null;
        hideFilterPopup();

        if (window.mosaicUtils) window.mosaicUtils.resizeAll();
    }

    function applyTechFilter(name) {
        if (!name) return;

        const projectsSection = document.querySelector('.projects-section');
        if (!projectsSection) return;

        currentFilter = name;

        document.querySelectorAll('.tech-item').forEach(item => {
            const label = item.querySelector('.tech-name')?.textContent || item.title || '';
            item.classList.toggle('filtered', label.toLowerCase() === name.toLowerCase());
        });

        let matchCount = 0;
        projectsSection.querySelectorAll('.project-card').forEach(card => {
            const tags = Array.from(card.querySelectorAll('.tech-tag'));
            const isMatch = tags.some(t => t.textContent.trim().toLowerCase() === name.toLowerCase());

            if (isMatch) {
                card.style.opacity = '1';
                card.style.transform = '';
                card.style.filter = '';
                matchCount++;
            } else {
                card.style.opacity = '0.25';
                card.style.transform = 'scale(0.96)';
                card.style.filter = 'grayscale(70%)';
            }
        });

        if (matchCount > 0) {
            showFilterPopup(name);
            projectsSection.scrollIntoView({behavior: 'smooth', block: 'start'});
        } else {
            clearTechFilter();
        }

        if (window.mosaicUtils) setTimeout(() => window.mosaicUtils.resizeAll(), 100);
    }

    function initTechFiltering() {
        const techSection = document.querySelector('.tech-section');
        const projectsSection = document.querySelector('.projects-section');

        if (!techSection || !projectsSection) return;

        techSection.querySelectorAll('.tech-item').forEach(item => {
            if (item.dataset.techFilterAttached) return;
            item.dataset.techFilterAttached = '1';

            const name = item.querySelector('.tech-name')?.textContent || item.title || '';
            if (!name) return;

            item.style.cursor = 'pointer';
            item.addEventListener('click', (e) => {
                e.preventDefault();
                e.stopPropagation();
                applyTechFilter(name);
            });
        });

        projectsSection.querySelectorAll('.tech-tag').forEach(tag => {
            if (tag.dataset.techFilterAttached) return;
            tag.dataset.techFilterAttached = '1';

            tag.style.cursor = 'pointer';
            tag.addEventListener('click', (e) => {
                e.preventDefault();
                e.stopPropagation();
                applyTechFilter(tag.textContent.trim());
            });
        });
    }

    window.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && currentFilter) clearTechFilter();
    });

    window.applyTechFilter = applyTechFilter;
    window.clearTechFilter = clearTechFilter;
    window.initTechFiltering = initTechFiltering;

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', () => setTimeout(initTechFiltering, 200));
    } else {
        setTimeout(initTechFiltering, 200);
    }
})();