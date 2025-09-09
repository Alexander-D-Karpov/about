function initSettingsEditor(pluginName, settings) {
    console.log(`Initializing settings editor for ${pluginName}:`, settings);

    const container = document.getElementById(`settings-${pluginName}`);
    if (!container) {
        console.error(`Container not found for plugin: ${pluginName}`);
        return;
    }

    container.innerHTML = '';

    // Validate and sanitize settings
    if (!settings || typeof settings !== 'object' || Array.isArray(settings)) {
        console.warn(`Invalid settings for ${pluginName}, using empty object:`, settings);
        settings = {};
    }

    // Check for corrupted settings (numeric string keys)
    const keys = Object.keys(settings);
    const hasNumericKeys = keys.some(key => !isNaN(key) && !isNaN(parseFloat(key)));

    if (hasNumericKeys) {
        console.warn(`Corrupted settings detected for ${pluginName}, clearing...`);
        settings = {};
    }

    // If settings is empty, show a message and basic structure
    if (Object.keys(settings).length === 0) {
        const emptyMessage = document.createElement('div');
        emptyMessage.className = 'empty-settings-message';
        emptyMessage.innerHTML = `
            <p>No settings configured for this plugin yet.</p>
            <button type="button" class="btn btn-secondary" onclick="addBasicSetting('${pluginName}')">
                Add Basic Setting
            </button>
        `;
        container.appendChild(emptyMessage);
        return;
    }

    function createField(key, value, parent = container, path = '') {
        const field = document.createElement('div');
        field.className = 'settings-field';

        const currentPath = path ? `${path}.${key}` : key;
        let fieldType = Array.isArray(value) ? 'array' : typeof value;

        if (value === null || value === undefined) {
            fieldType = 'string';
            value = '';
        }

        const header = document.createElement('div');
        header.className = 'field-header';

        const label = document.createElement('div');
        label.className = 'field-label';
        label.textContent = formatFieldName(key);
        header.appendChild(label);

        const typeLabel = document.createElement('div');
        typeLabel.className = 'field-type';
        typeLabel.textContent = fieldType;
        header.appendChild(typeLabel);

        field.appendChild(header);

        if (typeof value === 'string' || (value === null || value === undefined)) {
            const stringValue = value || '';
            let input;
            if (key.toLowerCase().includes('description') ||
                key.toLowerCase().includes('bio') ||
                stringValue.length > 100 ||
                key === 'ascii') {
                input = document.createElement('textarea');
                input.className = 'field-input field-textarea';
                input.rows = Math.min(Math.max(3, Math.ceil(stringValue.length / 50)), 10);
            } else {
                input = document.createElement('input');
                input.className = 'field-input';
                input.type = 'text';
            }

            input.value = stringValue;
            input.name = currentPath;
            input.placeholder = getPlaceholder(key);
            field.appendChild(input);

        } else if (typeof value === 'boolean') {
            const checkboxContainer = document.createElement('div');
            checkboxContainer.className = 'field-checkbox';

            const input = document.createElement('input');
            input.type = 'checkbox';
            input.checked = value;
            input.name = currentPath;
            input.id = `${pluginName}_${currentPath.replace(/\./g, '_')}`;

            const label = document.createElement('label');
            label.htmlFor = input.id;
            label.textContent = 'Enabled';

            checkboxContainer.appendChild(input);
            checkboxContainer.appendChild(label);
            field.appendChild(checkboxContainer);

        } else if (typeof value === 'number') {
            const input = document.createElement('input');
            input.className = 'field-input';
            input.type = 'number';
            input.value = value;
            input.name = currentPath;
            input.placeholder = getPlaceholder(key);
            field.appendChild(input);

        } else if (Array.isArray(value)) {
            const arrayContainer = document.createElement('div');
            arrayContainer.className = 'array-container';

            const arrayHeader = document.createElement('div');
            arrayHeader.className = 'array-header';

            const arrayTitle = document.createElement('div');
            arrayTitle.className = 'array-title';
            arrayTitle.textContent = `${formatFieldName(key)} (${value.length} items)`;
            arrayHeader.appendChild(arrayTitle);

            arrayContainer.appendChild(arrayHeader);

            value.forEach((item, index) => {
                createArrayItem(arrayContainer, item, currentPath, index);
            });

            const addBtn = document.createElement('button');
            addBtn.className = 'add-btn';
            addBtn.textContent = 'Add Item';
            addBtn.type = 'button';
            addBtn.onclick = () => addArrayItem(arrayContainer, currentPath);

            field.appendChild(arrayContainer);
            field.appendChild(addBtn);

        } else if (typeof value === 'object' && value !== null) {
            const objectContainer = document.createElement('div');
            objectContainer.className = 'object-container';

            Object.keys(value).forEach(nestedKey => {
                createField(nestedKey, value[nestedKey], objectContainer, currentPath);
            });

            field.appendChild(objectContainer);
        }

        parent.appendChild(field);
    }

    Object.keys(settings).forEach(key => {
        createField(key, settings[key]);
    });
}

function addBasicSetting(pluginName) {
    const container = document.getElementById(`settings-${pluginName}`);
    if (!container) return;

    // Clear empty message
    container.innerHTML = '';

    // Add a basic setting field
    const field = document.createElement('div');
    field.className = 'settings-field';

    field.innerHTML = `
        <div class="field-header">
            <div class="field-label">Setting Name</div>
            <div class="field-type">string</div>
        </div>
        <input class="field-input" type="text" name="newSetting" placeholder="Enter setting name..." />
    `;

    container.appendChild(field);
}

function createArrayItem(container, item, path, index) {
    const itemDiv = document.createElement('div');
    itemDiv.className = 'array-item';

    const itemContent = document.createElement('div');
    itemContent.className = 'array-item-content';

    if (typeof item === 'string') {
        const input = document.createElement('input');
        input.className = 'field-input';
        input.value = item;
        input.name = `${path}[${index}]`;
        input.placeholder = 'Enter value...';
        itemContent.appendChild(input);
    } else if (typeof item === 'object' && item !== null) {
        const textarea = document.createElement('textarea');
        textarea.className = 'field-input field-textarea';
        textarea.value = JSON.stringify(item, null, 2);
        textarea.name = `${path}[${index}]`;
        textarea.placeholder = 'Enter JSON object...';
        textarea.rows = Math.min(Math.max(4, Object.keys(item).length + 1), 10);
        itemContent.appendChild(textarea);
    } else {
        const input = document.createElement('input');
        input.className = 'field-input';
        input.value = String(item);
        input.name = `${path}[${index}]`;
        input.placeholder = 'Enter value...';
        itemContent.appendChild(input);
    }

    const controls = document.createElement('div');
    controls.className = 'array-item-controls';

    const removeBtn = document.createElement('button');
    removeBtn.className = 'remove-btn';
    removeBtn.textContent = 'Remove';
    removeBtn.type = 'button';
    removeBtn.onclick = () => {
        itemDiv.remove();
        updateArrayTitle(container);
        reindexArrayItems(container, path);
    };

    controls.appendChild(removeBtn);
    itemDiv.appendChild(itemContent);
    itemDiv.appendChild(controls);

    container.appendChild(itemDiv);
}

function reindexArrayItems(container, path) {
    const items = container.querySelectorAll('.array-item');
    items.forEach((item, newIndex) => {
        const input = item.querySelector('.field-input');
        if (input && input.name) {
            input.name = `${path}[${newIndex}]`;
        }
    });
}

function addArrayItem(container, path) {
    const index = container.querySelectorAll('.array-item').length;
    createArrayItem(container, '', path, index);
    updateArrayTitle(container);
}

function updateArrayTitle(container) {
    const title = container.querySelector('.array-title');
    if (title) {
        const count = container.querySelectorAll('.array-item').length;
        const fieldName = title.textContent.split(' (')[0];
        title.textContent = `${fieldName} (${count} items)`;
    }
}

function formatFieldName(key) {
    return key
        .replace(/([A-Z])/g, ' $1')
        .replace(/^./, str => str.toUpperCase())
        .replace(/_/g, ' ')
        .trim();
}

function getPlaceholder(key) {
    const placeholders = {
        url: 'https://example.com',
        email: 'user@example.com',
        name: 'Enter name...',
        title: 'Enter title...',
        description: 'Enter description...',
        bio: 'Enter biography...',
        username: 'Enter username...',
        apikey: 'Enter API key...',
        api_key: 'Enter API key...',
        token: 'Enter token...',
        password: 'Enter password...',
        steamid: 'Enter Steam ID...',
        image: '/static/images/example.jpg',
        icon: 'icon-name',
        sectiontitle: 'Section Title',
        webring_url: 'https://webring.example.com',
        sourcecodeur: 'https://github.com/user/repo'
    };

    const keyLower = key.toLowerCase();
    for (const [k, v] of Object.entries(placeholders)) {
        if (keyLower.includes(k)) return v;
    }

    return 'Enter value...';
}

function collectSettings(form) {
    const settings = {};
    const inputs = form.querySelectorAll('.field-input, input[type="checkbox"]');

    inputs.forEach(input => {
        const name = input.name;
        if (!name) return;

        let value = input.type === 'checkbox' ? input.checked : input.value;

        if (input.type === 'number') {
            value = parseFloat(value) || 0;
        } else if (input.tagName === 'TEXTAREA' && name.includes('[')) {
            try {
                const parsed = JSON.parse(value);
                if (typeof parsed === 'object') {
                    value = parsed;
                }
            } catch (e) {
                // Keep as string if not valid JSON
            }
        }

        setNestedValue(settings, name, value);
    });

    return settings;
}

function setNestedValue(obj, path, value) {
    const keys = path.split(/[\.\[\]]+/).filter(k => k !== '');
    let current = obj;

    for (let i = 0; i < keys.length - 1; i++) {
        const key = keys[i];
        const nextKey = keys[i + 1];

        if (!isNaN(nextKey)) {
            if (!(key in current)) current[key] = [];
            current = current[key];
        } else {
            if (!(key in current)) current[key] = {};
            current = current[key];
        }
    }

    const lastKey = keys[keys.length - 1];
    if (Array.isArray(current)) {
        const index = parseInt(lastKey);
        current[index] = value;
    } else {
        current[lastKey] = value;
    }
}

function initSortable() {
    new Sortable(document.getElementById('plugins-container'), {
        animation: 150,
        ghostClass: 'sortable-ghost',
        handle: '.plugin-header',
        onEnd: function(evt) {
            updatePluginOrder();
        }
    });
}

function initSearch() {
    const searchInput = document.getElementById('plugin-search');
    const plugins = document.querySelectorAll('.plugin');

    searchInput.addEventListener('input', (e) => {
        const query = e.target.value.toLowerCase();

        plugins.forEach(plugin => {
            const name = plugin.dataset.plugin.toLowerCase();
            const description = plugin.querySelector('.plugin-description').textContent.toLowerCase();
            const matches = name.includes(query) || description.includes(query);

            plugin.style.display = matches ? 'block' : 'none';
        });
    });
}

function updatePluginOrder() {
    const plugins = Array.from(document.querySelectorAll('.plugin'));
    const pluginsData = plugins.map((plugin, index) => {
        const pluginName = plugin.dataset.plugin;
        const form = plugin.querySelector('.plugin-form');
        const toggle = plugin.querySelector('.plugin-toggle');
        const orderInput = plugin.querySelector('.order-input');

        return {
            name: pluginName,
            enabled: toggle.checked,
            order: index,
            settings: collectSettings(form)
        };
    });

    fetch('/admin/api/plugins', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(pluginsData)
    })
        .then(response => response.json())
        .then(result => {
            if (result.success) {
                showNotification('Plugin order updated successfully!', 'success');
                plugins.forEach((plugin, index) => {
                    const orderInput = plugin.querySelector('.order-input');
                    if (orderInput) {
                        orderInput.value = index;
                    }
                });
            }
        })
        .catch(err => {
            showNotification('Failed to update order: ' + err.message, 'error');
        });
}

document.addEventListener('DOMContentLoaded', function() {
    document.querySelectorAll('.plugin-form').forEach(form => {
        form.addEventListener('submit', async function(e) {
            e.preventDefault();

            const pluginName = this.dataset.plugin;
            const submitBtn = this.querySelector('button[type="submit"]');
            const originalText = submitBtn.textContent;

            submitBtn.innerHTML = '<div class="loading"></div> Saving...';
            submitBtn.disabled = true;

            try {
                const formData = new FormData();
                const settings = collectSettings(this);
                const toggle = document.querySelector(`.plugin-toggle[data-plugin="${pluginName}"]`);
                const orderInput = this.querySelector('[name="order"]');

                formData.append('plugin', pluginName);
                formData.append('enabled', toggle.checked);
                formData.append('order', orderInput ? orderInput.value : '0');
                formData.append('settings', JSON.stringify(settings));

                const response = await fetch('/admin/api/plugin', {
                    method: 'POST',
                    body: formData
                });

                if (response.ok) {
                    const result = await response.json();
                    showNotification(result.message, 'success');
                } else {
                    const error = await response.text();
                    throw new Error(error);
                }
            } catch (err) {
                showNotification('Error: ' + err.message, 'error');
            } finally {
                submitBtn.textContent = originalText;
                submitBtn.disabled = false;
            }
        });
    });

    document.querySelectorAll('.plugin-toggle').forEach(toggle => {
        toggle.addEventListener('change', function() {
            const form = document.querySelector(`.plugin-form[data-plugin="${this.dataset.plugin}"]`);
            if (form) {
                form.dispatchEvent(new Event('submit', { bubbles: true }));
            }
        });
    });

    document.querySelectorAll('.order-input').forEach(input => {
        input.addEventListener('change', function() {
            const plugin = this.closest('.plugin');
            const form = plugin.querySelector('.plugin-form');
            if (form) {
                form.dispatchEvent(new Event('submit', { bubbles: true }));
            }
        });
    });

    document.querySelectorAll('.export-btn').forEach(btn => {
        btn.addEventListener('click', function() {
            const pluginName = this.dataset.plugin;
            const form = document.querySelector(`.plugin-form[data-plugin="${pluginName}"]`);
            const settings = collectSettings(form);

            const dataStr = JSON.stringify(settings, null, 2);
            const dataBlob = new Blob([dataStr], {type: 'application/json'});

            const link = document.createElement('a');
            link.href = URL.createObjectURL(dataBlob);
            link.download = `${pluginName}-settings.json`;
            link.click();

            showNotification(`${pluginName} settings exported!`, 'success');
        });
    });

    document.querySelectorAll('.reset-btn').forEach(btn => {
        btn.addEventListener('click', function() {
            const pluginName = this.dataset.plugin;
            if (confirm(`Reset ${pluginName} plugin settings to defaults?`)) {
                const form = document.querySelector(`.plugin-form[data-plugin="${pluginName}"]`);
                form.reset();
                showNotification(`${pluginName} settings reset!`, 'info');
            }
        });
    });
});

function showNotification(message, type) {
    const notification = document.createElement('div');
    notification.className = 'notification ' + type;
    notification.textContent = message;
    document.body.appendChild(notification);

    setTimeout(() => notification.classList.add('show'), 100);

    setTimeout(() => {
        notification.classList.remove('show');
        setTimeout(() => {
            if (notification.parentNode) {
                notification.parentNode.removeChild(notification);
            }
        }, 300);
    }, 3000);
}

function saveAllPlugins() {
    const forms = document.querySelectorAll('.plugin-form');
    let saved = 0;
    const total = forms.length;

    if (total === 0) {
        showNotification('No plugins to save!', 'info');
        return;
    }

    showNotification('Saving all plugins...', 'info');

    forms.forEach(form => {
        const handler = function() {
            saved++;
            if (saved === total) {
                showNotification('All plugins saved successfully!', 'success');
            }
            form.removeEventListener('submit', handler);
        };

        form.addEventListener('submit', handler, { once: true });
        form.dispatchEvent(new Event('submit', { bubbles: true }));
    });
}

function previewSite() {
    window.open('/', '_blank');
}

function refreshData() {
    showNotification('Refreshing data...', 'info');
    setTimeout(() => {
        window.location.reload();
    }, 1000);
}