import { useEffect, useMemo, useState } from "react";
import mainIcon from "../assets/main.png";
import type { JsonSettingObject, JsonSettingValue, SettingsConfigFile } from "../lib/bridge";

function isPlainObject(value: JsonSettingValue): value is JsonSettingObject {
    return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function isSelectSetting(value: JsonSettingValue): value is { active: string; list: string[] } {
    if (!isPlainObject(value)) {
        return false;
    }

    return (
        typeof value.active === "string" &&
        Array.isArray(value.list) &&
        value.list.every((item) => typeof item === "string")
    );
}

function cloneConfigs(configs: SettingsConfigFile[]) {
    return JSON.parse(JSON.stringify(configs)) as SettingsConfigFile[];
}

function settingTitle(value: string) {
    return value.replaceAll("_", " ");
}

export default function SettingsWindow() {
    const [configs, setConfigs] = useState<SettingsConfigFile[]>([]);
    const [activeConfigName, setActiveConfigName] = useState("");
    const [statusText, setStatusText] = useState("Loading settings...");
    const [isLoading, setIsLoading] = useState(true);
    const [isSaving, setIsSaving] = useState(false);

    const activeConfig = useMemo(
        () => configs.find((config) => config.name === activeConfigName) ?? configs[0] ?? null,
        [configs, activeConfigName],
    );

    useEffect(() => {
        function preventBrowserZoomByWheel(event: WheelEvent) {
            if (event.ctrlKey) {
                event.preventDefault();
            }
        }

        function preventBrowserZoomByKeyboard(event: KeyboardEvent) {
            if (!event.ctrlKey) {
                return;
            }

            if (
                event.key === "+" ||
                event.key === "-" ||
                event.key === "=" ||
                event.key === "0" ||
                event.code === "NumpadAdd" ||
                event.code === "NumpadSubtract" ||
                event.code === "Digit0"
            ) {
                event.preventDefault();
            }
        }

        window.addEventListener("wheel", preventBrowserZoomByWheel, {
            passive: false,
            capture: true,
        });

        window.addEventListener("keydown", preventBrowserZoomByKeyboard, {
            capture: true,
        });

        return () => {
            window.removeEventListener("wheel", preventBrowserZoomByWheel, {
                capture: true,
            });

            window.removeEventListener("keydown", preventBrowserZoomByKeyboard, {
                capture: true,
            });
        };
    }, []);

    useEffect(() => {
        void loadConfigs();
    }, []);

    function dragWindow() {
        window.settingsWindowDrag?.();
    }

    function minimizeWindow() {
        window.settingsWindowMinimize?.();
    }

    function closeWindow() {
        window.settingsWindowClose?.();
    }

    async function loadConfigs() {
        if (!window.loadSettingsConfigs) {
            setStatusText("Bridge loadSettingsConfigs недоступен");
            setIsLoading(false);
            return;
        }

        setIsLoading(true);

        try {
            const result = await window.loadSettingsConfigs();

            if (!result.ok) {
                setStatusText(result.message || "Failed to load settings");
                setConfigs([]);
                return;
            }

            const nextConfigs = result.configs ?? [];

            setConfigs(nextConfigs);
            setActiveConfigName(nextConfigs[0]?.name ?? "");
            setStatusText(nextConfigs.length > 0 ? "Settings loaded" : "JSON configs not found");
        } catch (error) {
            const message = error instanceof Error ? error.message : String(error);
            setStatusText(`Error loading settings: ${message}`);
        } finally {
            setIsLoading(false);
        }
    }

    function updateActiveConfig(updater: (draft: JsonSettingObject) => void) {
        setConfigs((current) => {
            const next = cloneConfigs(current);
            const target = next.find((config) => config.name === activeConfig?.name);

            if (target) {
                updater(target.data);
            }

            return next;
        });
    }

    function updateSelectSetting(blockKey: string, value: string) {
        updateActiveConfig((draft) => {
            const block = draft[blockKey];

            if (isSelectSetting(block)) {
                block.active = value;
            }
        });
    }

    function updateBlockField(blockKey: string, fieldKey: string, value: string | number | boolean) {
        updateActiveConfig((draft) => {
            const block = draft[blockKey];

            if (isPlainObject(block)) {
                block[fieldKey] = value;
            }
        });
    }

    async function saveActiveConfig() {
        if (!activeConfig) {
            setStatusText("There is no active config to save");
            return;
        }

        if (!window.saveSettingsConfig) {
            setStatusText("Bridge saveSettingsConfig недоступен");
            return;
        }

        setIsSaving(true);

        try {
            const result = await window.saveSettingsConfig(activeConfig.name, activeConfig.data);

            if (!result.ok) {
                setStatusText(result.message || "Failed to save settings");
                return;
            }

            setStatusText(result.message || "Settings saved");
        } catch (error) {
            const message = error instanceof Error ? error.message : String(error);
            setStatusText(`Saving error: ${message}`);
        } finally {
            setIsSaving(false);
        }
    }

    function renderPrimitiveSetting(blockKey: string, fieldKey: string, value: JsonSettingValue) {
        if (typeof value === "boolean") {
            return (
                <label key={fieldKey} className="settings-checkbox-row">
                    <input
                        type="checkbox"
                        checked={value}
                        onChange={(event) => updateBlockField(blockKey, fieldKey, event.target.checked)}
                    />
                    <span>{settingTitle(fieldKey)}</span>
                </label>
            );
        }

        if (typeof value === "number") {
            return (
                <label key={fieldKey} className="settings-field">
                    <span>{settingTitle(fieldKey)}</span>
                    <input
                        type="number"
                        value={value}
                        onChange={(event) => {
                            const parsed = Number(event.target.value);
                            updateBlockField(blockKey, fieldKey, Number.isNaN(parsed) ? 0 : parsed);
                        }}
                    />
                </label>
            );
        }

        if (typeof value === "string") {
            return (
                <label key={fieldKey} className="settings-field">
                    <span>{settingTitle(fieldKey)}</span>
                    <input
                        type="text"
                        value={value}
                        onChange={(event) => updateBlockField(blockKey, fieldKey, event.target.value)}
                    />
                </label>
            );
        }

        return null;
    }

    function renderBlock(blockKey: string, blockValue: JsonSettingValue) {
        if (isSelectSetting(blockValue)) {
            return (
                <section key={blockKey} className="settings-block">
                    <h2>{settingTitle(blockKey)}</h2>

                    <label className="settings-field">
                        <span>{settingTitle(blockKey)}</span>
                        <select
                            value={blockValue.active}
                            onChange={(event) => updateSelectSetting(blockKey, event.target.value)}
                        >
                            {blockValue.list.map((option) => (
                                <option key={option} value={option}>
                                    {option}
                                </option>
                            ))}
                        </select>
                    </label>
                </section>
            );
        }

        if (isPlainObject(blockValue)) {
            return (
                <section key={blockKey} className="settings-block">
                    <h2>{settingTitle(blockKey)}</h2>

                    <div className="settings-block__fields">
                        {Object.entries(blockValue)
                            .filter(([fieldKey]) => fieldKey !== "active" && fieldKey !== "list")
                            .map(([fieldKey, value]) => renderPrimitiveSetting(blockKey, fieldKey, value))}
                    </div>
                </section>
            );
        }

        return null;
    }

    return (
        <main className="settings-window" aria-label="Settings POSRelayv">
            <header
                className="settings-titlebar"
                aria-label="Window panel"
                onPointerDown={(event) => {
                    if (event.button === 0) {
                        dragWindow();
                    }
                }}
            >
                <div className="settings-titlebar__brand">
                    <img src={mainIcon} alt="" />
                    <span>POSRelayv</span>
                </div>

                <div className="settings-titlebar__actions">
                    <button
                        type="button"
                        aria-label="Collapse"
                        onPointerDown={(event) => event.stopPropagation()}
                        onClick={minimizeWindow}
                    >
                        −
                    </button>

                    <button
                        type="button"
                        aria-label="Closed"
                        onPointerDown={(event) => event.stopPropagation()}
                        onClick={closeWindow}
                    >
                        ×
                    </button>
                </div>
            </header>

            <section className="settings-layout">
                <aside className="settings-sidebar">
                    <div className="settings-sidebar__title">Settings</div>

                    {configs.map((config) => (
                        <button
                            key={config.name}
                            type="button"
                            className={activeConfig?.name === config.name ? "settings-nav-item settings-nav-item--active" : "settings-nav-item"}
                            onClick={() => {
                                setActiveConfigName(config.name);
                                setStatusText(`Load config: ${config.name}`);
                            }}
                        >
                            {config.name}
                        </button>
                    ))}
                </aside>

                <section className="settings-content-area">
                    <div className="settings-page-header">
                        <div>
                            <h1>{activeConfig?.name ?? "Settings"}</h1>
                            <p>{statusText}</p>
                        </div>
                    </div>

                    {isLoading ? (
                        <div className="settings-empty">Загрузка настроек...</div>
                    ) : configs.length === 0 ? (
                        <div className="settings-empty">В папке configs нет JSON-конфигов</div>
                    ) : (
                        <div className="settings-content">
                            {activeConfig ? Object.entries(activeConfig.data).map(([blockKey, blockValue]) => renderBlock(blockKey, blockValue)) : null}
                        </div>
                    )}

                    <footer className="settings-footer">
                        <button
                            type="button"
                            className="settings-secondary-button"
                            onClick={() => void loadConfigs()}
                            disabled={isLoading || isSaving}
                        >
                            Обновить
                        </button>

                        <button
                            type="button"
                            className="settings-save-button"
                            onClick={() => void saveActiveConfig()}
                            disabled={isLoading || isSaving || !activeConfig}
                        >
                            {isSaving ? "Saving..." : "Save"}
                        </button>
                    </footer>
                </section>
            </section>
        </main>
    );
}