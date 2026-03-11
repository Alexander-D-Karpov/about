let currentPluginData = {};
let fileUploadQueue = new Map();

function initSettingsEditor(pluginName, settings) {
    console.log(`Initializing settings editor for ${pluginName}:`, settings);

    currentPluginData[pluginName] = settings;

    const container = document.getElementById(`settings-${pluginName}`);
    if (!container) {
        console.error(`Container not found for plugin: ${pluginName}`);
        return;
    }

    container.innerHTML = '';

    if (!settings || Object.keys(settings).length === 0) {
        settings = createDefaultStructure(pluginName, getPluginType(pluginName));
    }

    renderPluginForm(container, pluginName, settings);
}

function renderPluginForm(container, pluginName, settings) {
    const pluginType = getPluginType(pluginName);
    const formWrapper = document.createElement('div');
    formWrapper.className = 'plugin-settings-wrapper';

    if (pluginName === 'photos') {
        renderPhotosAdmin(formWrapper, settings);
        container.appendChild(formWrapper);
        return;
    }

    if (pluginName === 'places') {
        renderPlacesPluginAdmin(formWrapper, pluginName, settings);
        container.appendChild(formWrapper);
        return;
    }

    if (pluginType.type === 'array-based') {
        renderArrayBasedPlugin(formWrapper, pluginName, settings, pluginType);
    } else {
        renderObjectBasedPlugin(formWrapper, pluginName, settings);
    }

    container.appendChild(formWrapper);
}

function renderPhotosAdmin(container, settings) {
    const uiSettings = settings.ui || {};
    createField('ui.sectionTitle', uiSettings.sectionTitle || 'Photos', container, 'ui');
    createField('ui.maxFolders', uiSettings.maxFolders || 6, container, 'ui');
    createField('apiUrl', settings.apiUrl || 'https://photos.akarpov.ru', container);

    const hiddenContainer = document.createElement('div');
    hiddenContainer.className = 'managed-array-container';
    hiddenContainer.innerHTML = `
        <div class="managed-array-header">
            <h4>Hidden Folders</h4>
            <p style="font-size:12px; color:var(--muted)">Enter folder names to hide from the main view.</p>
        </div>
    `;

    const namesList = settings.hiddenFolderNames || [];

    const arrayWrapper = document.createElement('div');
    renderGenericArray(arrayWrapper, namesList, 'hiddenFolderNames', 'photos');
    hiddenContainer.appendChild(arrayWrapper);

    container.appendChild(hiddenContainer);
}


function createDefaultStructure(pluginName, pluginType) {
    const baseStructure = {
        ui: {
            sectionTitle: getDefaultSectionTitle(pluginName)
        }
    };

    if (pluginType.type === 'array-based') {
        baseStructure[pluginType.arrayField] = [];
    }

    const specificDefaults = {
        'steam': { steamid: '' },
        'lastfm': { username: '' },
        'beatleader': { username: '' },
        'webring': { webring_url: '', username: '' },
        'code': { github: { username: '' }, wakatime: { api_key: '' } },
        'info': { sourceCodeURL: '' },
        'profile': {name: '', title: '', subtitle: '', bio: '', profileImage: ''},
        'places': {
            ui: {
                sectionTitle: 'Visited Places',
                showStats: true,
                defaultZoom: 2,
                defaultLat: 25,
                defaultLng: 0
            }
        },
        'health': {
            api: {
                baseUrl: 'https://api.hcgateway.shuchir.dev',
                username: '',
                password: ''
            },
            ui: {
                sectionTitle: 'Health Stats',
                showSteps: true,
                showCalories: true,
                showWorkouts: true,
                showSleep: true,
                showHeartRate: true,
                showHydration: true
            }
        },
    };

    if (specificDefaults[pluginName]) {
        Object.assign(baseStructure, specificDefaults[pluginName]);
    }

    return baseStructure;
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
                image: 'image',
                technologies: 'array'
            }
        },
        'neofetch': {
            arrayField: 'machines',
            itemSchema: {
                name: 'string',
                output: 'text'
            }
        },
        'services': {
            arrayField: 'services',
            itemSchema: {
                name: 'string',
                url: 'string',
                description: 'text',
                icon: 'image'
            }
        },
        'personal': {
            arrayField: 'info',
            itemSchema: {
                title: 'string',
                content: 'text',
                image: 'image',
                icon: 'string',
                category: 'string'
            }
        },
        'meme': {
            arrayField: 'memes',
            itemSchema: {
                text: 'string',
                image: 'image',
                type: 'select',
                source: 'string',
                category: 'string'
            }
        },
        'places': {
            arrayField: 'places',
            itemSchema: {
                name: 'string',
                lat: 'number',
                lng: 'number',
                country: 'string',
                city: 'string',
                description: 'text',
                visited_date: 'string',
                category: 'select'
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
            createField(key, settings[key], container, '', pluginName);
        }
    });

    const arrayData = settings[pluginType.arrayField] || [];
    renderManagedArray(container, pluginName, pluginType.arrayField, arrayData, pluginType.itemSchema);
}

function renderObjectBasedPlugin(container, pluginName, settings) {
    Object.keys(settings).forEach(key => {
        createField(key, settings[key], container, '', pluginName);
    });

    const addMoreBtn = document.createElement('button');
    addMoreBtn.type = 'button';
    addMoreBtn.className = 'btn btn-secondary add-setting-btn';
    addMoreBtn.textContent = 'Add Setting';
    addMoreBtn.onclick = () => addObjectSetting(container, pluginName);
    container.appendChild(addMoreBtn);
}

function createField(key, value, parent = document.body, path = '', pluginName = '') {
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
        input.step = 'any';
        input.value = value;
        input.name = currentPath;
        input.placeholder = getPlaceholder(key);
        field.appendChild(input);

    } else if (typeof value === 'string' || (value === null || value === undefined)) {
        const stringValue = value || '';

        if (isImageField(key, stringValue)) {
            createImageField(field, currentPath, stringValue, key, pluginName);
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
            renderGenericArray(field, value, currentPath, pluginName);
        }

    } else if (typeof value === 'object' && value !== null) {
        const objectContainer = document.createElement('div');
        objectContainer.className = 'object-container';
        objectContainer.style.marginLeft = '20px';
        objectContainer.style.borderLeft = '2px solid #ddd';
        objectContainer.style.paddingLeft = '15px';
        objectContainer.style.marginTop = '10px';

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

        Object.keys(value).forEach(nestedKey => {
            createField(nestedKey, value[nestedKey], objectContent, currentPath, pluginName);
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

function createImageField(field, currentPath, currentValue, key, pluginName) {
    const imageContainer = document.createElement('div');
    imageContainer.className = 'image-field-container';

    const isIconField = key.toLowerCase().includes('icon');
    const applyIconClass = (imgEl, val) => {
        const guessIcon = isIconField || isBareIconName(val) || (typeof val === 'string' && val.includes('/static/icons/'));
        imgEl.className = 'image-preview' + (guessIcon ? ' image-preview--icon' : '');
    };

    if (currentValue) {
        const preview = document.createElement('div');
        preview.className = 'current-image-preview';

        const img = document.createElement('img');
        img.src = resolveIconURL(currentValue);
        img.alt = `Current ${key}`;
        applyIconClass(img, currentValue);
        img.onerror = () => {
            img.style.display = 'none';
            preview.innerHTML += `<div style="color: var(--bad);">Image failed to load: ${resolveIconURL(currentValue)}</div>`;
        };

        const meta = document.createElement('div');
        meta.style.fontSize = '12px';
        meta.style.color = 'var(--muted)';
        meta.innerHTML = `
            <div>Value: <code>${currentValue}</code></div>
            ${isBareIconName(currentValue) ? `<div class="icon-default-hint">Default icon path: <code>${resolveIconURL(currentValue)}</code></div>` : ''}
        `;

        preview.appendChild(img);
        preview.appendChild(meta);
        imageContainer.appendChild(preview);
    }

    const fieldWithUpload = document.createElement('div');
    fieldWithUpload.className = 'field-with-upload';

    const textInput = document.createElement('input');
    textInput.className = 'field-input';
    textInput.type = 'text';
    textInput.value = currentValue || '';
    textInput.name = currentPath;
    textInput.placeholder = isIconField
        ? 'telegram or /static/icons/telegram.svg or uploaded URL'
        : 'Enter image path or upload file';

    textInput.addEventListener('input', () => {
        const preview = imageContainer.querySelector('.current-image-preview');
        if (!preview) return;

        let img = preview.querySelector('img');
        if (!img) {
            img = document.createElement('img');
            preview.prepend(img);
        }
        img.style.display = '';
        img.src = resolveIconURL(textInput.value);
        applyIconClass(img, textInput.value);

        const meta = preview.querySelector('div[style*="font-size: 12px"]');
        if (meta) {
            meta.innerHTML = `
                <div>Value: <code>${textInput.value}</code></div>
                ${isBareIconName(textInput.value) ? `<div class="icon-default-hint">Default icon path: <code>${resolveIconURL(textInput.value)}</code></div>` : ''}
            `;
        }
    });

    const fileInput = document.createElement('input');
    fileInput.type = 'file';
    fileInput.accept = 'image/*';
    fileInput.className = 'image-upload-input';
    fileInput.style.display = 'none';
    fileInput.id = `file-${currentPath.replace(/[\.\[\]]/g, '_')}`;

    const uploadBtn = document.createElement('button');
    uploadBtn.type = 'button';
    uploadBtn.className = 'btn btn-sm btn-secondary';
    uploadBtn.textContent = '📁 Upload';
    uploadBtn.onclick = () => fileInput.click();

    const progressDiv = document.createElement('div');
    progressDiv.className = 'upload-progress';
    progressDiv.textContent = 'Uploading...';

    fileInput.addEventListener('change', async (e) => {
        const file = e.target.files[0];
        if (!file) return;

        progressDiv.style.display = 'block';
        uploadBtn.disabled = true;
        try {
            const formData = new FormData();
            formData.append('file', file);
            formData.append('plugin', pluginName);
            formData.append('field', currentPath);

            const response = await fetch('/admin/api/upload', { method: 'POST', body: formData });
            if (!response.ok) throw new Error(await response.text());
            const result = await response.json();
            textInput.value = result.url;

            let preview = imageContainer.querySelector('.current-image-preview');
            if (!preview) {
                preview = document.createElement('div');
                preview.className = 'current-image-preview';
                imageContainer.insertBefore(preview, fieldWithUpload);
            }
            preview.innerHTML = '';

            const newImg = document.createElement('img');
            newImg.src = result.url;
            newImg.alt = `Current ${key}`;
            applyIconClass(newImg, result.url);

            const meta = document.createElement('div');
            meta.style.fontSize = '12px';
            meta.style.color = 'var(--muted)';
            meta.innerHTML = `<div>Value: <code>${result.url}</code></div>`;

            preview.appendChild(newImg);
            preview.appendChild(meta);

            showNotification('Image uploaded successfully!', 'success');
        } catch (err) {
            showNotification('Upload failed: ' + err.message, 'error');
        } finally {
            progressDiv.style.display = 'none';
            uploadBtn.disabled = false;
        }
    });

    fieldWithUpload.appendChild(textInput);
    fieldWithUpload.appendChild(uploadBtn);

    imageContainer.appendChild(fieldWithUpload);
    imageContainer.appendChild(fileInput);
    imageContainer.appendChild(progressDiv);

    field.appendChild(imageContainer);
}

function renderManagedArray(container, pluginName, fieldName, items, itemSchema) {
    const arrayContainer = document.createElement('div');
    arrayContainer.className = 'managed-array-container';

    const header = document.createElement('div');
    header.className = 'managed-array-header';
    header.innerHTML = `
        <h4>${formatFieldName(fieldName)} (<span class="item-count">${items.length}</span> items)</h4>
    `;
    arrayContainer.appendChild(header);

    const itemsContainer = document.createElement('div');
    itemsContainer.className = 'managed-array-items';
    itemsContainer.id = `managed-array-${fieldName}`;

    items.forEach((item, index) => {
        renderManagedArrayItem(itemsContainer, pluginName, fieldName, item, index, itemSchema);
    });

    arrayContainer.appendChild(itemsContainer);

    const footer = document.createElement('div');
    footer.className = 'managed-array-footer';

    const addBtn = document.createElement('button');
    addBtn.type = 'button';
    addBtn.className = 'btn btn-secondary add-array-item';
    addBtn.textContent = `Add ${fieldName.slice(0, -1)}`;
    addBtn.onclick = () => addManagedArrayItem(pluginName, fieldName);

    footer.appendChild(addBtn);
    arrayContainer.appendChild(footer);

    container.appendChild(arrayContainer);
}


function renderManagedArrayItem(container, pluginName, fieldName, item, index, itemSchema) {
    const itemDiv = document.createElement('div');
    itemDiv.className = 'managed-array-item';
    itemDiv.setAttribute('data-index', index);

    const itemHeader = document.createElement('div');
    itemHeader.className = 'managed-array-item-header';
    itemHeader.innerHTML = `
        <span class="item-number">#${index + 1}</span>
        <button type="button" class="btn btn-danger btn-sm remove-item" 
                onclick="removeManagedArrayItem(this, '${pluginName}', '${fieldName}')">Remove</button>
    `;

    itemDiv.appendChild(itemHeader);

    const itemContent = document.createElement('div');
    itemContent.className = 'managed-array-item-content';

    let latInput = null;
    let lngInput = null;
    let nameInput = null;

    Object.keys(itemSchema).forEach(key => {
        const value = item[key] !== undefined ? item[key] : getDefaultValueForType(itemSchema[key]);
        renderSchemaField(itemContent, `${fieldName}[${index}].${key}`, key, value, itemSchema[key], pluginName, `${fieldName}[${index}]`);

        if (pluginName === 'places') {
            const input = itemContent.querySelector(`[name="${fieldName}[${index}].${key}"]`);
            if (key === 'lat') latInput = input;
            if (key === 'lng') lngInput = input;
            if (key === 'name') nameInput = input;
        }
    });

    itemDiv.appendChild(itemContent);

    if (pluginName === 'places' && latInput && lngInput) {
        const mapContainer = document.createElement('div');
        mapContainer.className = 'admin-place-map-container';
        mapContainer.innerHTML = `
            <div class="admin-place-map-label">Click on map to set coordinates or search for a location:</div>
            <div class="admin-place-map" id="admin-place-map-${index}" style="height: 250px; border-radius: 8px; margin-top: 8px;"></div>
        `;
        itemDiv.appendChild(mapContainer);

        setTimeout(() => {
            const mapEl = document.getElementById(`admin-place-map-${index}`);
            if (mapEl) {
                initPlacesMapPicker(mapEl, latInput, lngInput, nameInput);
            }
        }, 100);
    }

    container.appendChild(itemDiv);
}

function renderSchemaField(container, fieldPath, fieldName, value, fieldType, pluginName, arrayPath) {
    const field = document.createElement('div');
    field.className = 'schema-field';

    const label = document.createElement('label');
    label.textContent = formatFieldName(fieldName);
    label.className = 'schema-field-label';
    field.appendChild(label);

    if (String(fieldName).toLowerCase() === 'icon') {
        createImageField(field, fieldPath, value, fieldName, pluginName);
        container.appendChild(field);
        return;
    }

    if (String(fieldName).toLowerCase() === 'output') {
        const textarea = document.createElement('textarea');
        textarea.className = 'field-input field-textarea neofetch-output-field';
        textarea.rows = 20;
        textarea.value = value || '';
        textarea.name = fieldPath;
        textarea.placeholder = 'Paste neofetch output here...';
        textarea.style.fontFamily = 'monospace';
        textarea.style.fontSize = '12px';
        textarea.style.whiteSpace = 'pre';
        textarea.style.overflowX = 'auto';
        field.appendChild(textarea);
        container.appendChild(field);
        return;
    }

    let input;

    switch (fieldType) {
        case 'text':
            input = document.createElement('textarea');
            input.className = 'field-input field-textarea';
            input.rows = 3;
            input.value = value || '';
            input.name = fieldPath;
            break;

        case 'number':
            input = document.createElement('input');
            input.className = 'field-input';
            input.type = 'number';
            input.step = 'any';
            input.value = value !== undefined && value !== null ? value : '';
            input.name = fieldPath;
            break;

        case 'date':
            input = document.createElement('input');
            input.className = 'field-input';
            input.type = 'date';
            input.value = value || '';
            input.name = fieldPath;
            break;

        case 'image':
            createImageField(field, fieldPath, value, fieldName, pluginName);
            container.appendChild(field);
            return;

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
            } else if (fieldName === 'category') {
                ['travel', 'home', 'work', 'vacation', 'adventure', 'food', 'culture', 'nature', 'other'].forEach(option => {
                    const opt = document.createElement('option');
                    opt.value = option;
                    opt.textContent = option.charAt(0).toUpperCase() + option.slice(1);
                    opt.selected = value === option;
                    input.appendChild(opt);
                });
            }
            input.name = fieldPath;
            break;

        case 'array':
            input = document.createElement('textarea');
            input.className = 'field-input field-textarea';
            input.rows = 2;
            input.placeholder = 'Enter comma-separated values';
            input.value = Array.isArray(value) ? value.join(', ') : (value || '');
            input.name = fieldPath;
            break;

        case 'object':
            input = document.createElement('textarea');
            input.className = 'field-input field-textarea';
            input.rows = 4;
            input.placeholder = 'Enter JSON object';
            input.value = typeof value === 'object' ? JSON.stringify(value, null, 2) : (value || '{}');
            input.name = fieldPath;
            break;

        default:
            input = document.createElement('input');
            input.className = 'field-input';
            input.type = 'text';
            input.value = value || '';
            input.name = fieldPath;
    }

    if (input) {
        input.placeholder = getPlaceholder(fieldName);
        field.appendChild(input);
    }

    container.appendChild(field);
}

function renderGenericArray(field, value, currentPath, pluginName) {
    const arrayContainer = document.createElement('div');
    arrayContainer.className = 'array-container';

    const arrayHeader = document.createElement('div');
    arrayHeader.className = 'array-header';
    arrayHeader.innerHTML = `<div class="array-title">Items (${value.length})</div>`;
    arrayContainer.appendChild(arrayHeader);

    value.forEach((item, index) => {
        createArrayItem(arrayContainer, item, currentPath, index, pluginName);
    });

    const addBtn = document.createElement('button');
    addBtn.className = 'add-btn btn btn-sm btn-secondary';
    addBtn.textContent = 'Add Item';
    addBtn.type = 'button';
    addBtn.onclick = () => addArrayItem(arrayContainer, currentPath, pluginName);

    field.appendChild(arrayContainer);
    field.appendChild(addBtn);
}

function createArrayItem(container, item, path, index, pluginName) {
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
        console.error(`Container not found for field ${fieldName}`);
        return;
    }

    const index = container.querySelectorAll('.managed-array-item').length;
    const pluginType = getPluginType(pluginName);
    const itemSchema = pluginType.itemSchema;

    const emptyItem = {};
    Object.keys(itemSchema).forEach(key => {
        emptyItem[key] = getDefaultValueForType(itemSchema[key]);
    });

    renderManagedArrayItem(container, pluginName, fieldName, emptyItem, index, itemSchema);
    updateManagedArrayTitle(pluginName, fieldName);
}

function removeManagedArrayItem(button, pluginName, fieldName) {
    const item = button.closest('.managed-array-item');
    const container = item.parentElement;
    item.remove();

    reindexManagedArrayItems(container);
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
                const parts = name.split('[');
                const fieldName = parts[0];
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

function addArrayItem(container, path, pluginName) {
    const index = container.querySelectorAll('.array-item').length;
    createArrayItem(container, '', path, index, pluginName);
    updateArrayTitle(container);
}

function updateManagedArrayTitle(pluginName, fieldName) {
    const container = document.getElementById(`managed-array-${fieldName}`);
    if (!container) return;

    const count = container.querySelectorAll('.managed-array-item').length;
    const countSpan = container.parentElement.querySelector('.managed-array-header .item-count');
    if (countSpan) countSpan.textContent = count;
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

    createField(settingName, defaultValue, container, '', pluginName);
}

function addNestedField(container, parentPath) {
    const fieldName = prompt('Enter field name:');
    if (!fieldName) return;

    const fieldType = prompt('Enter field type (string/number/boolean):', 'string');
    const defaultValue = getDefaultValueForType(fieldType);

    createField(fieldName, defaultValue, container, parentPath);
}

function collectSettings(form) {
    const settings = {};
    const inputs = form.querySelectorAll('.field-input, input[type="checkbox"]');

    inputs.forEach(input => {
        const name = input.name;
        if (!name) return;

        let value = input.type === 'checkbox' ? input.checked : input.value;

        if (input.type === 'number') {
            value = input.value === '' ? 0 : parseFloat(input.value);
            if (isNaN(value)) value = 0;
        } else if (input.tagName === 'TEXTAREA') {
            if (name.includes('ascii') || name.includes('colors')) {
                value = value.split('\n').filter(line => line.trim() !== '');
            } else if (name.includes('[') && name.includes(']')) {
                if (value.trim().startsWith('{') || value.trim().startsWith('[')) {
                    try {
                        value = JSON.parse(value);
                    } catch (e) {
                        if (name.includes('technologies')) {
                            value = value.split(',').map(tech => tech.trim()).filter(tech => tech !== '');
                        }
                    }
                } else if (name.includes('technologies')) {
                    value = value.split(',').map(tech => tech.trim()).filter(tech => tech !== '');
                }
            }
        }

        setNestedValue(settings, name, value);
    });

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

                if (input.type === 'number') {
                    value = input.value === '' ? 0 : parseFloat(input.value);
                    if (isNaN(value)) value = 0;
                } else if (input.tagName === 'TEXTAREA' && fieldName === 'technologies') {
                    value = value.split(',').map(tech => tech.trim()).filter(tech => tech !== '');
                }

                itemData[fieldName] = value;
            });

            if (Object.keys(itemData).length > 0) {
                settings[arrayName][index] = itemData;
            }
        });
    });

    const pluginName = form.dataset.plugin;
    if (pluginName === 'places') {
        const fromState =
            (currentPluginData[pluginName] && Array.isArray(currentPluginData[pluginName].places))
                ? currentPluginData[pluginName].places
                : (Array.isArray(placesData) ? placesData : []);

        settings.places = fromState;
    }

    return settings;
}

function setNestedValue(obj, path, value) {
    const keys = path.split(/[\.\[\]]+/).filter(k => k !== '');
    let current = obj;

    for (let i = 0; i < keys.length - 1; i++) {
        const key = keys[i];
        const nextKey = keys[i + 1];

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

function isImageField(key, value) {
    const imageKeys = ['image', 'profileimage', 'avatar', 'cover', 'icon', 'logo', 'photo', 'picture', 'imagecropped', 'favicon'];
    const keyLower = key.toLowerCase();

    const isImageKey = imageKeys.some(imgKey => keyLower.includes(imgKey));

    const isImageValue = typeof value === 'string' &&
        (value.match(/\.(jpg|jpeg|png|gif|webp|svg|ico)$/i) ||
            value.includes('/static/') ||
            value.includes('/media/') ||
            value.includes('http://') ||
            value.includes('https://'));

    return isImageKey || isImageValue;
}

function getDefaultValueForType(fieldType) {
    switch (fieldType) {
        case 'array': return [];
        case 'object': return {};
        case 'number': return 0;
        case 'boolean': return false;
        case 'text': return '';
        case 'image': return '';
        case 'date':
            return '';
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
        steamid: 'Enter Steam ID...',
        image: '/static/images/example.jpg',
        icon: 'telegram or /static/icons/telegram.svg',
        sectiontitle: 'Section Title',
        webring_url: 'https://webring.example.com',
        sourcecodeur: 'https://github.com/user/repo',
        hostname: 'localhost',
        github: 'https://github.com/user/repo',
        live: 'https://example.com',
        text: 'Enter text...',
        source: 'Source...',
        category: 'Category',
        refreshInterval: '300',
        lat: 'Latitude (e.g. 51.5074)',
        lng: 'Longitude (e.g. -0.1278)',
        country: 'Country name',
        city: 'City name',
        visited_date: 'YYYY-MM-DD'
    };

    const keyLower = key.toLowerCase();
    for (const [k, v] of Object.entries(placeholders)) {
        if (keyLower.includes(k)) return v;
    }

    return 'Enter value...';
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
        'meme': 'Random Meme',
        'places': 'Visited Places',
        'health': 'Health Stats',
    };

    return titles[pluginName] || formatFieldName(pluginName);
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

                    currentPluginData[pluginName] = result.settings || settings;

                    const container = document.getElementById(`settings-${pluginName}`);
                    if (container) {
                        container.innerHTML = '';
                        initSettingsEditor(pluginName, currentPluginData[pluginName]);
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
    notification.className = `notification ${type}`;
    notification.textContent = message;

    Object.assign(notification.style, {
        position: 'fixed',
        top: '20px',
        right: '20px',
        padding: '8px 16px',
        borderRadius: '4px',
        color: 'white',
        fontWeight: '500',
        zIndex: '10000',
        transform: 'translateX(400px)',
        transition: 'transform 0.3s ease',
        maxWidth: '300px'
    });

    switch (type) {
        case 'success':
            notification.style.background = '#4CAF50';
            break;
        case 'error':
            notification.style.background = '#f44336';
            break;
        case 'info':
            notification.style.background = '#2196F3';
            break;
        default:
            notification.style.background = '#333';
    }

    document.body.appendChild(notification);

    setTimeout(() => {
        notification.style.transform = 'translateX(0)';
    }, 100);

    setTimeout(() => {
        notification.style.transform = 'translateX(400px)';
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
        form.dispatchEvent(new Event('submit', { bubbles: true }));
        saved++;
        if (saved === total) {
            setTimeout(() => {
                showNotification('All plugins saved successfully!', 'success');
            }, 1000);
        }
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

async function reloadPlugin(pluginName) {
    try {
        const response = await fetch(`/admin/api/plugin/reload?plugin=${pluginName}`);
        if (response.ok) {
            const data = await response.json();

            currentPluginData[pluginName] = data.settings;

            const container = document.getElementById(`settings-${pluginName}`);
            if (container) {
                container.innerHTML = '';
                initSettingsEditor(pluginName, data.settings);
            }

            const toggle = document.querySelector(`.plugin-toggle[data-plugin="${pluginName}"]`);
            if (toggle) {
                toggle.checked = data.enabled;
            }

            const orderInput = document.querySelector(`.plugin[data-plugin="${pluginName}"] .order-input`);
            if (orderInput) {
                orderInput.value = data.order;
            }

            showNotification(`Plugin ${pluginName} reloaded successfully`, 'success');
        } else {
            throw new Error('Failed to reload plugin');
        }
    } catch (err) {
        showNotification('Error reloading plugin: ' + err.message, 'error');
    }
}

async function reloadConfig() {
    try {
        const response = await fetch('/admin/api/refresh', {
            method: 'POST'
        });

        if (response.ok) {
            showNotification('Configuration reloaded from disk', 'success');
            setTimeout(() => {
                window.location.reload();
            }, 1000);
        } else {
            throw new Error('Failed to reload configuration');
        }
    } catch (err) {
        showNotification('Error: ' + err.message, 'error');
    }
}

function isBareIconName(val) {
    return typeof val === 'string' && val.trim() !== '' && !/[\/.]/.test(val);
}
function resolveIconURL(val) {
    if (!val) return '';
    return isBareIconName(val) ? `/static/icons/${val}.svg` : val;
}

let placesAdminMap = null;
let placesMarkers = new Map();
let selectedPlaceIndex = null;
let placesData = [];

function initPlacesAdminMap(pluginName) {
    const container = document.getElementById(`places-admin-map-${pluginName}`);
    if (!container) return;

    if (placesAdminMap) {
        placesAdminMap.remove();
        placesAdminMap = null;
    }

    placesMarkers.clear();

    placesAdminMap = L.map(container, {
        center: [25, 0],
        zoom: 2,
        zoomControl: true
    });

    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
        attribution: '&copy; OpenStreetMap',
        maxZoom: 18,
        errorTileUrl: 'https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png'
    }).on('tileerror', function () {
        this.setUrl('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png');
    }).addTo(placesAdminMap);

    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
        attribution: '&copy; OSM &copy; CARTO',
        maxZoom: 18
    }).addTo(placesAdminMap);

    loadPlacesFromSettings(pluginName);

    placesAdminMap.on('click', function (e) {
        addNewPlace(e.latlng, pluginName);
    });

    const searchInput = document.getElementById(`places-search-${pluginName}`);
    const searchBtn = document.getElementById(`places-search-btn-${pluginName}`);

    if (searchInput && searchBtn) {
        const doSearch = () => {
            const query = searchInput.value.trim();
            if (!query) return;

            searchBtn.disabled = true;
            searchBtn.textContent = '...';

            fetch(`https://nominatim.openstreetmap.org/search?format=json&q=${encodeURIComponent(query)}&limit=1`)
                .then(r => r.json())
                .then(results => {
                    if (results && results.length > 0) {
                        const result = results[0];
                        const lat = parseFloat(result.lat);
                        const lng = parseFloat(result.lon);

                        placesAdminMap.setView([lat, lng], 12);

                        addNewPlace({lat, lng}, pluginName, result.display_name.split(',')[0]);
                    } else {
                        showNotification('Location not found', 'error');
                    }
                })
                .catch(() => showNotification('Search failed', 'error'))
                .finally(() => {
                    searchBtn.disabled = false;
                    searchBtn.textContent = 'Search';
                });
        };

        searchBtn.onclick = doSearch;
        searchInput.onkeypress = (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                doSearch();
            }
        };
    }
}

function loadPlacesFromSettings(pluginName) {
    const settings = currentPluginData[pluginName] || {};
    placesData = settings.places || [];

    updatePlacesSettings(pluginName);

    renderPlacesList(pluginName);
    renderPlacesOnMap(pluginName);

    if (placesData.length > 0) {
        const bounds = L.latLngBounds(placesData.map(p => [p.lat, p.lng]));
        placesAdminMap.fitBounds(bounds, {padding: [50, 50], maxZoom: 10});
    }
}


function renderPlacesOnMap(pluginName) {
    placesMarkers.forEach(marker => marker.remove());
    placesMarkers.clear();

    placesData.forEach((place, index) => {
        if (place.lat && place.lng) {
            const marker = L.circleMarker([place.lat, place.lng], {
                radius: 10,
                fillColor: selectedPlaceIndex === index ? '#3ad38b' : '#7aa2ff',
                color: '#ffffff',
                weight: 3,
                opacity: 1,
                fillOpacity: 0.9
            }).addTo(placesAdminMap);

            marker.on('click', () => {
                selectPlace(index, pluginName);
            });

            marker.bindTooltip(place.name || `Place ${index + 1}`, {
                permanent: false,
                direction: 'top'
            });

            placesMarkers.set(index, marker);
        }
    });
}

function renderPlacesList(pluginName) {
    const listContainer = document.getElementById(`places-list-${pluginName}`);
    if (!listContainer) return;

    if (placesData.length === 0) {
        listContainer.innerHTML = `
            <div style="padding: 20px; text-align: center; color: var(--muted);">
                <p>No places added yet.</p>
                <p style="font-size: 12px;">Click on the map to add a place.</p>
            </div>
        `;
        return;
    }

    listContainer.innerHTML = placesData.map((place, index) => `
        <div class="places-list-item ${selectedPlaceIndex === index ? 'selected' : ''}" 
             data-index="${index}" onclick="selectPlace(${index}, '${pluginName}')">
            <div class="places-list-item-info">
                <div class="places-list-item-name">${place.name || `Place ${index + 1}`}</div>
                <div class="places-list-item-location">
                    ${[place.city, place.country].filter(Boolean).join(', ') || `${place.lat?.toFixed(4)}, ${place.lng?.toFixed(4)}`}
                </div>
            </div>
            <div class="places-list-item-actions">
                <button type="button" class="btn btn-sm btn-secondary" 
                        onclick="event.stopPropagation(); focusPlaceOnMap(${index})">📍</button>
                <button type="button" class="btn btn-sm btn-danger" 
                        onclick="event.stopPropagation(); deletePlace(${index}, '${pluginName}')">✕</button>
            </div>
        </div>
    `).join('');
}

function selectPlace(index, pluginName) {
    selectedPlaceIndex = index;

    renderPlacesList(pluginName);
    renderPlacesOnMap(pluginName);
    renderPlaceEditForm(pluginName);

    focusPlaceOnMap(index);
}

function focusPlaceOnMap(index) {
    const place = placesData[index];
    if (place && place.lat && place.lng && placesAdminMap) {
        placesAdminMap.setView([place.lat, place.lng], 12);

        const marker = placesMarkers.get(index);
        if (marker) {
            marker.openTooltip();
        }
    }
}

function addNewPlace(latlng, pluginName, suggestedName = '') {
    const newPlace = {
        name: suggestedName || '',
        lat: parseFloat(latlng.lat.toFixed(6)),
        lng: parseFloat(latlng.lng.toFixed(6)),
        country: '',
        city: '',
        description: '',
        visited_date: '',
        category: 'travel'
    };

    placesData.push(newPlace);
    selectedPlaceIndex = placesData.length - 1;

    renderPlacesList(pluginName);
    renderPlacesOnMap(pluginName);
    renderPlaceEditForm(pluginName);

    if (!suggestedName) {
        reverseGeocodePlace(selectedPlaceIndex, pluginName);
    }

    updatePlacesSettings(pluginName);
}

function reverseGeocodePlace(index, pluginName) {
    const place = placesData[index];
    if (!place) return;

    fetch(`https://nominatim.openstreetmap.org/reverse?format=json&lat=${place.lat}&lon=${place.lng}`)
        .then(r => r.json())
        .then(result => {
            if (result && result.address) {
                const addr = result.address;

                if (!place.name) {
                    place.name = addr.tourism || addr.amenity || addr.building ||
                        addr.city || addr.town || addr.village ||
                        addr.suburb || addr.county || '';
                }

                place.country = addr.country || '';
                place.city = addr.city || addr.town || addr.village || '';

                if (selectedPlaceIndex === index) {
                    renderPlaceEditForm(pluginName);
                }
                renderPlacesList(pluginName);
                updatePlacesSettings(pluginName);
            }
        })
        .catch(() => {
        });
}

function renderPlaceEditForm(pluginName) {
    const formContainer = document.getElementById(`place-edit-form-${pluginName}`);
    if (!formContainer) return;

    if (selectedPlaceIndex === null || !placesData[selectedPlaceIndex]) {
        formContainer.innerHTML = `
            <div style="padding: 20px; text-align: center; color: var(--muted);">
                Select a place from the list or click on the map to add one.
            </div>
        `;
        return;
    }

    const place = placesData[selectedPlaceIndex];
    const categories = ['travel', 'home', 'work', 'vacation', 'adventure', 'food', 'culture', 'nature', 'other'];

    formContainer.innerHTML = `
        <div class="place-edit-panel">
            <h4>Edit Place #${selectedPlaceIndex + 1}</h4>
            
            <div class="coord-display">
                <span>📍</span>
                <span>Lat: ${place.lat?.toFixed(6) || '0'}</span>
                <span>Lng: ${place.lng?.toFixed(6) || '0'}</span>
                <button type="button" class="btn btn-sm btn-secondary" 
                        onclick="updatePlaceFromMapClick('${pluginName}')"
                        title="Click to update coordinates from map">
                    Update from map
                </button>
            </div>
            
            <div class="place-edit-grid" style="margin-top: 12px;">
                <div class="place-edit-field full-width">
                    <label>Name</label>
                    <input type="text" id="place-name-${pluginName}" value="${escapeHtml(place.name || '')}" 
                           placeholder="Place name" onchange="updatePlaceField('${pluginName}', 'name', this.value)">
                </div>
                
                <div class="place-edit-field">
                    <label>Country</label>
                    <input type="text" id="place-country-${pluginName}" value="${escapeHtml(place.country || '')}" 
                           placeholder="Country" onchange="updatePlaceField('${pluginName}', 'country', this.value)">
                </div>
                
                <div class="place-edit-field">
                    <label>City</label>
                    <input type="text" id="place-city-${pluginName}" value="${escapeHtml(place.city || '')}" 
                           placeholder="City" onchange="updatePlaceField('${pluginName}', 'city', this.value)">
                </div>
                
                <div class="place-edit-field">
                    <label>Category</label>
                    <select id="place-category-${pluginName}" onchange="updatePlaceField('${pluginName}', 'category', this.value)">
                        ${categories.map(cat => `
                            <option value="${cat}" ${place.category === cat ? 'selected' : ''}>
                                ${cat.charAt(0).toUpperCase() + cat.slice(1)}
                            </option>
                        `).join('')}
                    </select>
                </div>
                
                <div class="place-edit-field">
                    <label>Visited Date</label>
                    <input type="date" id="place-date-${pluginName}" value="${place.visited_date || ''}" 
                           onchange="updatePlaceField('${pluginName}', 'visited_date', this.value)">
                </div>
                
                <div class="place-edit-field full-width">
                    <label>Description</label>
                    <textarea id="place-desc-${pluginName}" placeholder="Description..." 
                              onchange="updatePlaceField('${pluginName}', 'description', this.value)">${escapeHtml(place.description || '')}</textarea>
                </div>
            </div>
            
            <div class="place-edit-actions">
                <button type="button" class="btn btn-secondary" onclick="deselectPlace('${pluginName}')">
                    Done
                </button>
                <button type="button" class="btn btn-danger" onclick="deletePlace(${selectedPlaceIndex}, '${pluginName}')">
                    Delete Place
                </button>
            </div>
        </div>
    `;
}

function updatePlaceField(pluginName, field, value) {
    if (selectedPlaceIndex === null || !placesData[selectedPlaceIndex]) return;

    placesData[selectedPlaceIndex][field] = value;
    renderPlacesList(pluginName);
    updatePlacesSettings(pluginName);
}

function updatePlaceFromMapClick(pluginName) {
    if (selectedPlaceIndex === null) return;

    showNotification('Click on the map to set new coordinates', 'info');

    const handler = (e) => {
        placesData[selectedPlaceIndex].lat = parseFloat(e.latlng.lat.toFixed(6));
        placesData[selectedPlaceIndex].lng = parseFloat(e.latlng.lng.toFixed(6));

        renderPlaceEditForm(pluginName);
        renderPlacesOnMap(pluginName);
        updatePlacesSettings(pluginName);

        placesAdminMap.off('click', handler);

        reverseGeocodePlace(selectedPlaceIndex, pluginName);
    };

    placesAdminMap.once('click', handler);
}

function deletePlace(index, pluginName) {
    if (!confirm('Delete this place?')) return;

    placesData.splice(index, 1);

    if (selectedPlaceIndex === index) {
        selectedPlaceIndex = null;
    } else if (selectedPlaceIndex > index) {
        selectedPlaceIndex--;
    }

    renderPlacesList(pluginName);
    renderPlacesOnMap(pluginName);
    renderPlaceEditForm(pluginName);
    updatePlacesSettings(pluginName);
}

function deselectPlace(pluginName) {
    selectedPlaceIndex = null;
    renderPlacesList(pluginName);
    renderPlacesOnMap(pluginName);
    renderPlaceEditForm(pluginName);
}

function updatePlacesSettings(pluginName) {
    if (!currentPluginData[pluginName]) {
        currentPluginData[pluginName] = {};
    }
    currentPluginData[pluginName].places = placesData;
}

function escapeHtml(text) {
    if (!text) return '';
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

function renderPlacesPluginAdmin(container, pluginName, settings) {
    container.innerHTML = '';

    Object.keys(settings).forEach(key => {
        if (key !== 'places') {
            createField(key, settings[key], container, '', pluginName);
        }
    });

    const placesContainer = document.createElement('div');
    placesContainer.className = 'places-admin-container';
    placesContainer.innerHTML = `
        <div class="places-admin-instructions">
            <p><strong>Places Editor</strong></p>
            <ul>
                <li>Click on the map to add a new place</li>
                <li>Click on a marker or list item to edit</li>
                <li>Use the search to find locations</li>
                <li>Drag markers to reposition (coming soon)</li>
            </ul>
        </div>
        
        <div class="places-admin-search">
            <input type="text" id="places-search-${pluginName}" placeholder="Search for a location...">
            <button type="button" class="btn btn-secondary" id="places-search-btn-${pluginName}">Search</button>
        </div>
        
        <div class="places-admin-map-wrapper">
            <div class="places-admin-map" id="places-admin-map-${pluginName}"></div>
        </div>
        
        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 16px; margin-top: 16px;">
            <div>
                <h4 style="margin: 0 0 8px 0; font-size: 13px; font-weight: 700;">Places List</h4>
                <div class="places-list-panel" id="places-list-${pluginName}"></div>
            </div>
            <div id="place-edit-form-${pluginName}"></div>
        </div>
    `;

    container.appendChild(placesContainer);

    setTimeout(() => {
        loadLeafletAndInit(pluginName);
    }, 100);
}

function loadLeafletAndInit(pluginName) {
    if (window.L) {
        initPlacesAdminMap(pluginName);
        return;
    }

    const cssLink = document.createElement('link');
    cssLink.rel = 'stylesheet';
    cssLink.href = '/static/libs/leaflet/leaflet.css';
    document.head.appendChild(cssLink);

    const script = document.createElement('script');
    script.src = '/static/libs/leaflet/leaflet.js';
    script.onload = () => initPlacesAdminMap(pluginName);
    script.onerror = () => {
        script.src = 'https://unpkg.com/leaflet@1.9.4/dist/leaflet.js';
        cssLink.href = 'https://unpkg.com/leaflet@1.9.4/dist/leaflet.css';
    };
    document.head.appendChild(script);
}

window.selectPlace = selectPlace;
window.focusPlaceOnMap = focusPlaceOnMap;
window.deletePlace = deletePlace;
window.deselectPlace = deselectPlace;
window.updatePlaceField = updatePlaceField;
window.updatePlaceFromMapClick = updatePlaceFromMapClick;
window.addManagedArrayItem = addManagedArrayItem;
window.removeManagedArrayItem = removeManagedArrayItem;
window.saveAllPlugins = saveAllPlugins;
window.previewSite = previewSite;
window.refreshData = refreshData;
window.reloadPlugin = reloadPlugin;
window.reloadConfig = reloadConfig;
window.initSettingsEditor = initSettingsEditor;