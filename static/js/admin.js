function initSettingsEditor(pluginName, settings) {
    console.log(`Initializing settings editor for ${pluginName}:`, settings);

    const container = document.getElementById(`settings-${pluginName}`);
    if (!container) {
        console.error(`Container not found for plugin: ${pluginName}`);
        return;
    }

    container.innerHTML = '';

    // Use the settings exactly as provided from the file
    // Only create minimal structure if completely missing
    if (!settings || Object.keys(settings).length === 0) {
        console.log(`No settings found for ${pluginName}, creating minimal structure`);
        settings = createDefaultStructure(pluginName, getPluginType(pluginName));
    }

    console.log(`Final settings for ${pluginName}:`, settings);
    renderPluginForm(container, pluginName, settings);
}

function renderPluginForm(container, pluginName, settings) {
    const pluginType = getPluginType(pluginName);

    const formWrapper = document.createElement('div');
    formWrapper.className = 'plugin-settings-wrapper';

    // Always use existing settings as base, never replace with defaults
    if (pluginType.type === 'array-based') {
        renderArrayBasedPlugin(formWrapper, pluginName, settings, pluginType);
    } else {
        renderObjectBasedPlugin(formWrapper, pluginName, settings);
    }

    container.appendChild(formWrapper);
}

function createDefaultStructure(pluginName, pluginType) {
    // Only create minimal UI structure, preserve all existing data
    const baseStructure = {
        ui: {
            sectionTitle: getDefaultSectionTitle(pluginName)
        }
    };

    // For array-based plugins, only add empty array if needed
    if (pluginType.type === 'array-based') {
        baseStructure[pluginType.arrayField] = [];
    }

    // For specific plugins that need nested config, add minimal structure
    if (pluginName === 'steam') {
        baseStructure.steamid = '';
    }

    if (pluginName === 'lastfm') {
        baseStructure.username = '';
    }

    if (pluginName === 'beatleader') {
        baseStructure.username = '';
    }

    if (pluginName === 'webring') {
        baseStructure.webring_url = '';
        baseStructure.username = '';
    }

    if (pluginName === 'code') {
        baseStructure.github = { username: '' };
        baseStructure.wakatime = { api_key: '' };
    }

    return baseStructure;
}

function getPluginUIDefaults(pluginName) {
    const defaults = {
        'profile': {},
        'social': {},
        'techstack': {},
        'projects': {},
        'lastfm': {
            showScrobbles: true,
            showPlayButton: true,
            showRecentTracks: true
        },
        'beatleader': {
            showPepeGif: true,
            showRecentMaps: true,
            showMainStats: true,
            loadingText: 'Loading BeatLeader data...'
        },
        'steam': {},
        'neofetch': {},
        'webring': {},
        'visitors': {
            showTotal: true,
            showToday: true,
            showVisitors: true
        },
        'services': {
            showStatus: true,
            showResponseTime: true
        },
        'code': {
            showGitHub: true,
            showWakatime: true,
            showLanguages: true,
            showCommitGraph: true
        },
        'info': {
            showServerInfo: true,
            showBuildInfo: false,
            showSourceCode: true,
            showSystemInfo: false
        },
        'personal': {
            showImages: true,
            showCategories: true,
            layout: 'grid'
        },
        'meme': {
            showMeme: true,
            autoRefresh: false,
            refreshInterval: 300
        }
    };

    return defaults[pluginName] || {};
}

function getDefaultSectionTitle(pluginName) {
    const titles = {
        'profile': 'Profile',
        'social': 'Links',
        'techstack': 'Technologies',
        'projects': 'Projects',
        'lastfm': 'Music',
        'beatleader': 'BeatLeader Stats',
        'steam': 'Gaming Activity',
        'neofetch': 'System Information',
        'webring': 'webring',
        'visitors': 'Visitors',
        'services': 'Local Services',
        'code': 'Coding Stats',
        'info': 'Page Info',
        'personal': 'Personal Info',
        'meme': 'Random Meme'
    };

    return titles[pluginName] || formatFieldName(pluginName);
}

function getPluginType(pluginName) {
    const arrayBasedPlugins = {
        'social': {
            arrayField: 'links',
            itemSchema: {
                name: 'string',
                url: 'string',
                icon: 'string',
                iconPath: 'string'
            }
        },
        'techstack': {
            arrayField: 'technologies',
            itemSchema: {
                name: 'string',
                icon: 'string',
                iconPath: 'string'
            }
        },
        'projects': {
            arrayField: 'projects',
            itemSchema: {
                name: 'string',
                description: 'text',
                github: 'string',
                live: 'string',
                image: 'string',
                technologies: 'array'
            }
        },
        'neofetch': {
            arrayField: 'machines',
            itemSchema: {
                name: 'string',
                username: 'string',
                hostname: 'string',
                ascii: 'array',
                info: 'object',
                colors: 'array'
            }
        },
        'services': {
            arrayField: 'services',
            itemSchema: {
                name: 'string',
                url: 'string',
                description: 'text',
                icon: 'string'
            }
        },
        'personal': {
            arrayField: 'info',
            itemSchema: {
                title: 'string',
                content: 'text',
                image: 'string',
                icon: 'string',
                category: 'string'
            }
        },
        'meme': {
            arrayField: 'memes',
            itemSchema: {
                text: 'string',
                image: 'string',
                type: 'select',
                source: 'string',
                category: 'string'
            }
        }
    };

    if (arrayBasedPlugins[pluginName]) {
        return { type: 'array-based', ...arrayBasedPlugins[pluginName] };
    }

    return { type: 'object-based' };
}

function renderArrayBasedPlugin(container, pluginName, settings, pluginType) {
    Object.keys(settings).forEach(key => {
        if (key !== pluginType.arrayField) {
            createField(key, settings[key], container);
        }
    });

    const arrayData = settings[pluginType.arrayField] || [];

    if (Array.isArray(arrayData) && arrayData.length > 0) {
        renderManagedArray(container, pluginName, pluginType.arrayField, arrayData, pluginType.itemSchema);
    } else {
        const emptyArrayContainer = document.createElement('div');
        emptyArrayContainer.className = 'managed-array-container';

        const header = document.createElement('div');
        header.className = 'managed-array-header';
        header.innerHTML = `
            <h4>${formatFieldName(pluginType.arrayField)} (0 items)</h4>
            <button type="button" class="btn btn-secondary add-array-item" 
                    onclick="addManagedArrayItem('${pluginName}', '${pluginType.arrayField}')">
                Add ${pluginType.arrayField.slice(0, -1)}
            </button>
        `;

        const itemsContainer = document.createElement('div');
        itemsContainer.className = 'managed-array-items';
        itemsContainer.id = `managed-array-${pluginType.arrayField}`;

        emptyArrayContainer.appendChild(header);
        emptyArrayContainer.appendChild(itemsContainer);
        container.appendChild(emptyArrayContainer);
    }
}

function renderObjectBasedPlugin(container, pluginName, settings) {
    // Always render all existing settings from the file
    Object.keys(settings).forEach(key => {
        createField(key, settings[key], container);
    });

    // Add plugin-specific fields that might be missing but are essential
    const requiredFields = getRequiredFieldsForPlugin(pluginName);
    requiredFields.forEach(fieldConfig => {
        if (!settings.hasOwnProperty(fieldConfig.key)) {
            createField(fieldConfig.key, fieldConfig.defaultValue, container);
        }
    });

    const addMoreBtn = document.createElement('button');
    addMoreBtn.type = 'button';
    addMoreBtn.className = 'btn btn-secondary add-setting-btn';
    addMoreBtn.textContent = 'Add Setting';
    addMoreBtn.onclick = () => addObjectSetting(container, pluginName);
    container.appendChild(addMoreBtn);
}

function getRequiredFieldsForPlugin(pluginName) {
    const requiredFields = {
        'steam': [
            { key: 'steamid', defaultValue: '' }
        ],
        'lastfm': [
            { key: 'username', defaultValue: '' }
        ],
        'beatleader': [
            { key: 'username', defaultValue: '' }
        ],
        'webring': [
            { key: 'webring_url', defaultValue: '' },
            { key: 'username', defaultValue: '' }
        ],
        'code': [
            { key: 'github', defaultValue: { username: '' } },
            { key: 'wakatime', defaultValue: { api_key: '' } }
        ],
        'info': [
            { key: 'sourceCodeURL', defaultValue: '' }
        ]
    };

    return requiredFields[pluginName] || [];
}

function renderManagedArray(container, pluginName, fieldName, items, itemSchema) {
    const arrayContainer = document.createElement('div');
    arrayContainer.className = 'managed-array-container';

    const header = document.createElement('div');
    header.className = 'managed-array-header';
    header.innerHTML = `
        <h4>${formatFieldName(fieldName)} (${items.length} items)</h4>
        <button type="button" class="btn btn-secondary add-array-item" 
                onclick="addManagedArrayItem('${pluginName}', '${fieldName}')">
            Add ${fieldName.slice(0, -1)}
        </button>
    `;

    arrayContainer.appendChild(header);

    const itemsContainer = document.createElement('div');
    itemsContainer.className = 'managed-array-items';
    itemsContainer.id = `managed-array-${fieldName}`;

    items.forEach((item, index) => {
        renderManagedArrayItem(itemsContainer, fieldName, item, index, itemSchema);
    });

    arrayContainer.appendChild(itemsContainer);
    container.appendChild(arrayContainer);
}

function renderManagedArrayItem(container, fieldName, item, index, itemSchema) {
    const itemDiv = document.createElement('div');
    itemDiv.className = 'managed-array-item';
    itemDiv.setAttribute('data-index', index);

    const itemHeader = document.createElement('div');
    itemHeader.className = 'managed-array-item-header';
    itemHeader.innerHTML = `
        <span class="item-number">#${index + 1}</span>
        <button type="button" class="btn btn-danger btn-sm remove-item" 
                onclick="removeManagedArrayItem(this)">Remove</button>
    `;

    itemDiv.appendChild(itemHeader);

    const itemContent = document.createElement('div');
    itemContent.className = 'managed-array-item-content';

    Object.keys(itemSchema).forEach(key => {
        const value = item[key] !== undefined ? item[key] : getDefaultValueForType(itemSchema[key]);
        renderSchemaField(itemContent, `${fieldName}[${index}].${key}`, key, value, itemSchema[key]);
    });

    itemDiv.appendChild(itemContent);
    container.appendChild(itemDiv);
}

function renderSchemaField(container, fieldPath, fieldName, value, fieldType) {
    const field = document.createElement('div');
    field.className = 'schema-field';

    const label = document.createElement('label');
    label.textContent = formatFieldName(fieldName);
    label.className = 'schema-field-label';
    field.appendChild(label);

    let input;

    switch (fieldType) {
        case 'text':
            input = document.createElement('textarea');
            input.className = 'field-input field-textarea';
            input.rows = 3;
            break;
        case 'select':
            input = document.createElement('select');
            input.className = 'field-input';
            if (fieldName === 'type') {
                ['image', 'gif', 'text'].forEach(option => {
                    const opt = document.createElement('option');
                    opt.value = option;
                    opt.textContent = option;
                    opt.selected = value === option;
                    input.appendChild(opt);
                });
            }
            break;
        case 'array':
            input = document.createElement('textarea');
            input.className = 'field-input field-textarea';
            input.rows = 2;
            input.placeholder = 'Enter comma-separated values';
            value = Array.isArray(value) ? value.join(', ') : (value || '');
            break;
        case 'object':
            input = document.createElement('textarea');
            input.className = 'field-input field-textarea';
            input.rows = 4;
            input.placeholder = 'Enter JSON object';
            value = typeof value === 'object' ? JSON.stringify(value, null, 2) : (value || '{}');
            break;
        default:
            input = document.createElement('input');
            input.className = 'field-input';
            input.type = 'text';
    }

    input.name = fieldPath;
    input.value = value !== undefined ? String(value) : '';
    input.placeholder = getPlaceholder(fieldName);

    field.appendChild(input);
    container.appendChild(field);
}

function createField(key, value, parent = document.body, path = '') {
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

    if (typeof value === 'boolean') {
        const checkboxContainer = document.createElement('div');
        checkboxContainer.className = 'field-checkbox';

        const input = document.createElement('input');
        input.type = 'checkbox';
        input.checked = value;
        input.name = currentPath;
        input.id = `${currentPath.replace(/\./g, '_')}`;

        const label = document.createElement('label');
        label.htmlFor = input.id;
        label.textContent = formatFieldName(key);

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

    } else if (typeof value === 'string' || (value === null || value === undefined)) {
        const stringValue = value || '';

        // Check if this is an image field
        if (isImageField(key, stringValue)) {
            createImageField(field, currentPath, stringValue, key);
        } else if (key.toLowerCase().includes('description') ||
            key.toLowerCase().includes('bio') ||
            key.toLowerCase().includes('content') ||
            key.toLowerCase().includes('message') ||
            stringValue.length > 100) {
            const textarea = document.createElement('textarea');
            textarea.className = 'field-input field-textarea';
            textarea.value = stringValue;
            textarea.name = currentPath;
            textarea.placeholder = getPlaceholder(key);
            textarea.rows = Math.min(Math.max(3, Math.ceil(stringValue.length / 50)), 8);
            field.appendChild(textarea);
        } else {
            const input = document.createElement('input');
            input.className = 'field-input';
            input.type = 'text';
            input.value = stringValue;
            input.name = currentPath;
            input.placeholder = getPlaceholder(key);
            field.appendChild(input);
        }

    } else if (Array.isArray(value)) {
        if (key === 'ascii' || key === 'colors') {
            const textarea = document.createElement('textarea');
            textarea.className = 'field-input field-textarea';
            textarea.value = value.join('\n');
            textarea.name = currentPath;
            textarea.rows = Math.min(value.length + 2, 12);
            textarea.placeholder = key === 'ascii' ? 'Enter ASCII art lines' : 'Enter colors (one per line)';
            field.appendChild(textarea);
        } else {
            renderGenericArray(field, value, currentPath);
        }

    } else if (typeof value === 'object' && value !== null) {
        // Recursively create fields for nested objects
        const objectContainer = document.createElement('div');
        objectContainer.className = 'object-container';
        objectContainer.style.marginLeft = '20px';
        objectContainer.style.borderLeft = '2px solid #ddd';
        objectContainer.style.paddingLeft = '15px';
        objectContainer.style.marginTop = '10px';

        // Create collapsible header for nested objects
        const toggleBtn = document.createElement('button');
        toggleBtn.type = 'button';
        toggleBtn.className = 'object-toggle-btn';
        toggleBtn.textContent = '▼ ';
        toggleBtn.style.background = 'none';
        toggleBtn.style.border = 'none';
        toggleBtn.style.cursor = 'pointer';
        toggleBtn.style.fontSize = '12px';

        const objectLabel = document.createElement('span');
        objectLabel.textContent = `${formatFieldName(key)} (${Object.keys(value).length} properties)`;
        objectLabel.style.fontWeight = 'bold';
        objectLabel.style.color = '#666';

        const objectHeader = document.createElement('div');
        objectHeader.className = 'object-header';
        objectHeader.style.marginBottom = '10px';
        objectHeader.appendChild(toggleBtn);
        objectHeader.appendChild(objectLabel);

        const objectContent = document.createElement('div');
        objectContent.className = 'object-content';

        // Recursively create fields for all nested properties
        Object.keys(value).forEach(nestedKey => {
            createField(nestedKey, value[nestedKey], objectContent, currentPath);
        });

        toggleBtn.addEventListener('click', () => {
            const isCollapsed = objectContent.style.display === 'none';
            objectContent.style.display = isCollapsed ? 'block' : 'none';
            toggleBtn.textContent = isCollapsed ? '▼ ' : '▶ ';
        });

        const addNestedBtn = document.createElement('button');
        addNestedBtn.type = 'button';
        addNestedBtn.className = 'btn btn-sm btn-secondary';
        addNestedBtn.textContent = `Add to ${formatFieldName(key)}`;
        addNestedBtn.style.marginTop = '10px';
        addNestedBtn.onclick = () => addNestedField(objectContent, currentPath);

        objectContainer.appendChild(objectHeader);
        objectContainer.appendChild(objectContent);
        objectContainer.appendChild(addNestedBtn);
        field.appendChild(objectContainer);
    }

    parent.appendChild(field);
}

function isImageField(key, value) {
    const imageKeys = ['image', 'profileimage', 'avatar', 'cover', 'icon', 'logo', 'photo', 'picture'];
    const keyLower = key.toLowerCase();

    // Check if key name suggests it's an image field
    const isImageKey = imageKeys.some(imgKey => keyLower.includes(imgKey));

    // Check if value looks like an image path
    const isImageValue = typeof value === 'string' &&
        (value.match(/\.(jpg|jpeg|png|gif|webp|svg)$/i) ||
            value.includes('/static/images/') ||
            value.includes('/media/'));

    return isImageKey || isImageValue;
}

function createImageField(field, currentPath, currentValue, key) {
    const imageContainer = document.createElement('div');
    imageContainer.className = 'image-field-container';

    // Current image preview
    if (currentValue) {
        const preview = document.createElement('div');
        preview.className = 'current-image-preview';
        preview.innerHTML = `
            <img src="${currentValue}" alt="Current ${key}" style="max-width: 100px; max-height: 100px; object-fit: cover; border: 1px solid #ddd; border-radius: 4px;">
            <div style="font-size: 12px; color: #666; margin-top: 4px;">Current: ${currentValue}</div>
        `;
        imageContainer.appendChild(preview);
    }

    // Text input for manual path entry
    const textInput = document.createElement('input');
    textInput.className = 'field-input';
    textInput.type = 'text';
    textInput.value = currentValue || '';
    textInput.name = currentPath;
    textInput.placeholder = 'Enter image path or upload file';
    textInput.style.marginBottom = '8px';

    // File upload input
    const fileInput = document.createElement('input');
    fileInput.type = 'file';
    fileInput.accept = 'image/*';
    fileInput.className = 'image-upload-input';
    fileInput.style.marginBottom = '8px';

    // Upload button
    const uploadBtn = document.createElement('button');
    uploadBtn.type = 'button';
    uploadBtn.className = 'btn btn-sm btn-secondary';
    uploadBtn.textContent = 'Upload New Image';
    uploadBtn.onclick = () => fileInput.click();

    fileInput.addEventListener('change', async (e) => {
        const file = e.target.files[0];
        if (file) {
            try {
                const formData = new FormData();
                formData.append('file', file);

                const response = await fetch('/admin/api/upload', {
                    method: 'POST',
                    body: formData
                });

                if (response.ok) {
                    const result = await response.json();
                    textInput.value = result.url;

                    // Update preview
                    const preview = imageContainer.querySelector('.current-image-preview');
                    if (preview) {
                        preview.innerHTML = `
                            <img src="${result.url}" alt="Current ${key}" style="max-width: 100px; max-height: 100px; object-fit: cover; border: 1px solid #ddd; border-radius: 4px;">
                            <div style="font-size: 12px; color: #666; margin-top: 4px;">Current: ${result.url}</div>
                        `;
                    }

                    showNotification('Image uploaded successfully!', 'success');
                } else {
                    const error = await response.text();
                    showNotification('Upload failed: ' + error, 'error');
                }
            } catch (err) {
                showNotification('Upload error: ' + err.message, 'error');
            }
        }
    });

    imageContainer.appendChild(textInput);
    imageContainer.appendChild(fileInput);
    imageContainer.appendChild(uploadBtn);
    field.appendChild(imageContainer);
}

function renderGenericArray(field, value, currentPath) {
    const arrayContainer = document.createElement('div');
    arrayContainer.className = 'array-container';

    const arrayHeader = document.createElement('div');
    arrayHeader.className = 'array-header';
    arrayHeader.innerHTML = `<div class="array-title">Items (${value.length})</div>`;
    arrayContainer.appendChild(arrayHeader);

    value.forEach((item, index) => {
        createArrayItem(arrayContainer, item, currentPath, index);
    });

    const addBtn = document.createElement('button');
    addBtn.className = 'add-btn btn btn-sm btn-secondary';
    addBtn.textContent = 'Add Item';
    addBtn.type = 'button';
    addBtn.onclick = () => addArrayItem(arrayContainer, currentPath);

    field.appendChild(arrayContainer);
    field.appendChild(addBtn);
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
        textarea.rows = Math.min(Math.max(3, Object.keys(item).length + 1), 8);
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
    removeBtn.className = 'btn btn-sm btn-danger remove-btn';
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

function addManagedArrayItem(pluginName, fieldName) {
    let container = document.getElementById(`managed-array-${fieldName}`);

    if (!container) {
        const pluginContainer = document.getElementById(`settings-${pluginName}`);
        const emptySection = pluginContainer.querySelector('.managed-array-container');

        if (emptySection) {
            const itemsContainer = document.createElement('div');
            itemsContainer.className = 'managed-array-items';
            itemsContainer.id = `managed-array-${fieldName}`;
            emptySection.appendChild(itemsContainer);
            container = itemsContainer;
        } else {
            return;
        }
    }

    const index = container.querySelectorAll('.managed-array-item').length;
    const pluginType = getPluginType(pluginName);
    const itemSchema = pluginType.itemSchema;

    const emptyItem = {};
    Object.keys(itemSchema).forEach(key => {
        emptyItem[key] = getDefaultValueForType(itemSchema[key]);
    });

    renderManagedArrayItem(container, fieldName, emptyItem, index, itemSchema);
    updateManagedArrayTitle(pluginName, fieldName);
}


function removeManagedArrayItem(button) {
    const item = button.closest('.managed-array-item');
    const container = item.parentElement;
    item.remove();

    reindexManagedArrayItems(container);

    const fieldName = container.id.replace('managed-array-', '');
    const pluginName = container.closest('.plugin').dataset.plugin;
    updateManagedArrayTitle(pluginName, fieldName);
}

function reindexManagedArrayItems(container) {
    const items = container.querySelectorAll('.managed-array-item');
    items.forEach((item, newIndex) => {
        item.setAttribute('data-index', newIndex);
        const itemNumber = item.querySelector('.item-number');
        if (itemNumber) {
            itemNumber.textContent = `#${newIndex + 1}`;
        }

        const inputs = item.querySelectorAll('.field-input');
        inputs.forEach(input => {
            const name = input.name;
            if (name && name.includes('[') && name.includes(']')) {
                const fieldName = name.split('[')[0];
                const propertyName = name.split('.').pop();
                input.name = `${fieldName}[${newIndex}].${propertyName}`;
            }
        });
    });
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

function updateManagedArrayTitle(pluginName, fieldName) {
    const container = document.getElementById(`managed-array-${fieldName}`);
    if (!container) return;

    const header = container.parentElement.querySelector('.managed-array-header h4');
    if (header) {
        const count = container.querySelectorAll('.managed-array-item').length;
        header.textContent = `${formatFieldName(fieldName)} (${count} items)`;
    }
}

function updateArrayTitle(container) {
    const title = container.querySelector('.array-title');
    if (title) {
        const count = container.querySelectorAll('.array-item').length;
        title.textContent = `Items (${count})`;
    }
}

function addObjectSetting(container, pluginName) {
    const settingName = prompt('Enter setting name:');
    if (!settingName) return;

    const settingType = prompt('Enter setting type (string/number/boolean/object/array):', 'string');
    const defaultValue = getDefaultValueForType(settingType);

    createField(settingName, defaultValue, container);
}

function addNestedField(container, parentPath) {
    const fieldName = prompt('Enter field name:');
    if (!fieldName) return;

    const fieldType = prompt('Enter field type (string/number/boolean):', 'string');
    const defaultValue = getDefaultValueForType(fieldType);

    createField(fieldName, defaultValue, container, parentPath);
}

function getDefaultValueForType(fieldType) {
    switch (fieldType) {
        case 'array': return [];
        case 'object': return {};
        case 'number': return 0;
        case 'boolean': return false;
        case 'text': return '';
        default: return '';
    }
}

function formatFieldName(key) {
    return key
        .replace(/([A-Z])/g, ' $1')
        .replace(/^./, str => str.toUpperCase())
        .replace(/_/g, ' ')
        .replace(/([a-z])([A-Z])/g, '$1 $2')
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
        content: 'Enter content...',
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
        sourcecodeur: 'https://github.com/user/repo',
        hostname: 'localhost',
        github: 'https://github.com/user/repo',
        live: 'https://example.com',
        text: 'Enter text...',
        source: 'Source...',
        category: 'Category',
        refreshInterval: '300'
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

        // Handle different input types without double-escaping
        if (input.type === 'number') {
            value = parseFloat(value) || 0;
        } else if (input.tagName === 'TEXTAREA') {
            if (name.includes('ascii') || name.includes('colors')) {
                // Handle ASCII art and color arrays
                value = value.split('\n').filter(line => line.trim() !== '');
            } else if (name.includes('[') && name.includes(']')) {
                // Handle array items - check if it's JSON or comma-separated
                if (value.trim().startsWith('{') || value.trim().startsWith('[')) {
                    try {
                        value = JSON.parse(value);
                    } catch (e) {
                        // If JSON parsing fails, treat as comma-separated for technologies
                        if (name.includes('technologies')) {
                            value = value.split(',').map(tech => tech.trim()).filter(tech => tech !== '');
                        }
                    }
                } else if (name.includes('technologies')) {
                    // Handle comma-separated technologies
                    value = value.split(',').map(tech => tech.trim()).filter(tech => tech !== '');
                }
            }
            // For other textareas, keep as string - DO NOT escape or modify
        }
        // For regular text inputs, keep value as-is without escaping

        setNestedValue(settings, name, value);
    });

    // Handle managed arrays (for array-based plugins)
    const managedArrays = form.querySelectorAll('.managed-array-items');
    managedArrays.forEach(arrayContainer => {
        const arrayName = arrayContainer.id.replace('managed-array-', '');
        const items = arrayContainer.querySelectorAll('.managed-array-item');

        if (!settings[arrayName]) {
            settings[arrayName] = [];
        }

        items.forEach((item, index) => {
            const itemData = {};
            const itemInputs = item.querySelectorAll('.field-input');

            itemInputs.forEach(input => {
                const fieldName = input.name.split('.').pop();
                let value = input.value;

                if (input.tagName === 'TEXTAREA' && fieldName === 'technologies') {
                    value = value.split(',').map(tech => tech.trim()).filter(tech => tech !== '');
                }

                itemData[fieldName] = value;
            });

            if (Object.keys(itemData).length > 0) {
                settings[arrayName][index] = itemData;
            }
        });
    });

    return settings;
}

function setNestedValue(obj, path, value) {
    const keys = path.split(/[\.\[\]]+/).filter(k => k !== '');
    let current = obj;

    for (let i = 0; i < keys.length - 1; i++) {
        const key = keys[i];
        const nextKey = keys[i + 1];

        // Check if the next key is a number (array index)
        if (!isNaN(nextKey) && nextKey !== '') {
            if (!(key in current)) current[key] = [];
            if (!Array.isArray(current[key])) current[key] = [];
            current = current[key];
        } else {
            if (!(key in current)) current[key] = {};
            if (typeof current[key] !== 'object' || Array.isArray(current[key])) {
                current[key] = {};
            }
            current = current[key];
        }
    }

    const lastKey = keys[keys.length - 1];
    if (Array.isArray(current)) {
        const index = parseInt(lastKey);
        if (!isNaN(index)) {
            current[index] = value;
        }
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
                const orderInput = document.querySelector(`.plugin[data-plugin="${pluginName}"] .order-input`);

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

                    const pluginContainer = this.closest('.plugin');
                    if (pluginContainer) {
                        pluginContainer.dataset.order = result.order || orderInput.value;
                    }

                    const settingsContainer = document.getElementById(`settings-${pluginName}`);
                    if (settingsContainer) {
                        const currentSettings = collectSettings(this);
                        settingsContainer.innerHTML = '';
                        initSettingsEditor(pluginName, currentSettings);
                    }
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
                plugin.dataset.order = this.value;
                form.dispatchEvent(new Event('submit', { bubbles: true }));
                updatePluginOrder();
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