(function () {
    "use strict";

    if (!document.body.classList.contains("dashboard-body")) {
        return;
    }

    const routes = [
        ["Vue générale", [["⌂", "Tableau de bord", "/go/dashboard"]]],
        ["Supervision", [["◫", "Stockage", "/storage"], ["●", "Services", "/services"], ["↔", "Réseau", "/network"], ["≡", "Journaux", "/logs"]]],
        ["Services", [["◆", "Apache", "/apache"], ["PHP", "PHP", "/php"], ["DB", "MySQL", "/mysql"], ["◉", "Tor", "/tor"], ["▦", "Fail2ban", "/fail2ban"], ["▤", "Pare-feu", "/firewall"], ["◷", "Cron", "/cron"], ["◇", "Certificats TLS", "/certbot"]]],
        ["Administration", [["↑", "Mises à jour", "/updates"], ["♙", "Utilisateurs", "/users"], ["▦", "Modules", "/modules"], ["⚙", "Paramètres", "/setting"], ["≣", "Config. serveur", "/configuration"], ["i", "À propos", "/about"]]]
    ];

    const main = document.querySelector("main.dashboard-shell");
    if (!main) return;

    const currentPath = window.location.pathname.replace(/\/$/, "") || "/";
    const themeStylesheet = document.createElement("link");
    themeStylesheet.rel = "stylesheet";
    themeStylesheet.href = `/assets/themes.css?v=${document.querySelector('link[href*="?v="]')?.href.split("?v=")[1] || ""}`;
    document.head.appendChild(themeStylesheet);
    const storedTheme = document.cookie.match(/(?:^|; )aegisadmin_theme=([^;]+)/)?.[1];
    const activeTheme = ["dark", "light", "bootstrap"].includes(storedTheme) ? storedTheme : "dark";
    document.documentElement.dataset.theme = activeTheme;
    const shell = document.createElement("div");
    shell.className = "app-shell";

    const header = document.createElement("header");
    header.className = "app-header";
    header.innerHTML = `
        <a class="app-header__brand" href="/go/dashboard">
            <span class="app-brand__mark" aria-hidden="true">
                <img class="app-brand__logo" src="/assets/aegisadmin-mark.png?v=${document.querySelector('link[href*="?v="]')?.href.split("?v=")[1] || ""}" alt="">
            </span>
            <span class="app-brand__identity"><strong class="app-brand__name">AegisAdmin</strong><small class="app-brand__version">Version ${document.querySelector('link[href*="?v="]')?.href.split("?v=")[1] || ""}</small></span>
        </a>
        <button class="app-header__menu-button" type="button" aria-label="Ouvrir le menu" aria-expanded="false"><span>☰</span><span>Menu</span></button>
        <div class="app-header__actions">
            <label class="app-theme-selector"><span class="app-theme-selector__label">Thème</span><select class="app-theme-selector__select" aria-label="Thème"><option value="dark">Sombre</option><option value="light">Clair</option><option value="bootstrap">Bootstrap</option></select></label>
            <span class="app-header__time"></span><button class="app-header__button" type="button">Actualiser</button>
        </div>`;

    const sidebar = document.createElement("aside");
    sidebar.className = "app-sidebar";
    sidebar.id = "app-sidebar";
    sidebar.setAttribute("aria-label", "Menu principal");
    const sidebarClose = document.createElement("button");
    sidebarClose.className = "app-sidebar__close";
    sidebarClose.type = "button";
    sidebarClose.setAttribute("aria-label", "Fermer le menu");
    sidebarClose.textContent = "× Fermer le menu";
    const nav = document.createElement("nav");
    nav.className = "app-navigation";
    routes.forEach(([category, links]) => {
        const group = document.createElement("div");
        group.className = "app-navigation__group";
        const title = document.createElement("button");
        title.type = "button";
        title.className = "app-navigation__section";
        title.innerHTML = '<span></span><span class="app-navigation__chevron" aria-hidden="true">›</span>';
        title.firstElementChild.textContent = category;
        const submenu = document.createElement("div");
        submenu.className = "app-navigation__submenu";
        let categoryActive = false;
        links.forEach(([icon, label, href]) => {
            const link = document.createElement("a");
            link.className = "app-navigation__link";
            if (currentPath === href || (href !== "/go/dashboard" && currentPath.startsWith(href + "/"))) {
                link.classList.add("is-active");
                link.setAttribute("aria-current", "page");
                categoryActive = true;
            }
            link.href = href;
            link.innerHTML = `<span class="app-navigation__icon" aria-hidden="true"></span><span></span>`;
            link.children[0].textContent = icon;
            link.children[1].textContent = label;
            submenu.appendChild(link);
        });
        group.classList.toggle("is-open", categoryActive);
        title.setAttribute("aria-expanded", String(categoryActive));
        submenu.hidden = !categoryActive;
        title.addEventListener("click", () => {
            const open = !group.classList.contains("is-open");
			if (open) {
				nav.querySelectorAll(".app-navigation__group.is-open").forEach((other) => {
					if (other === group) return;
					other.classList.remove("is-open");
					other.querySelector(".app-navigation__section")?.setAttribute("aria-expanded", "false");
					const otherSubmenu = other.querySelector(".app-navigation__submenu");
					if (otherSubmenu) otherSubmenu.hidden = true;
				});
			}
            group.classList.toggle("is-open", open);
            title.setAttribute("aria-expanded", String(open));
            submenu.hidden = !open;
        });
        group.append(title, submenu);
        nav.appendChild(group);
    });

    const userMenu = document.createElement("div");
    userMenu.className = "app-user-menu";
    userMenu.innerHTML = '<a class="app-user-menu__login" href="/go/account/password">Mon compte</a>';
    const logout = main.querySelector('form[action="/logout"]');
    if (logout) {
        logout.querySelector("button")?.classList.add("button", "button--neutral", "button--sm");
        userMenu.appendChild(logout);
    }
    nav.prepend(userMenu);
    fetch("/go/navigation", {headers: {"Accept": "application/json"}, credentials: "same-origin"})
        .then((response) => response.ok ? response.json() : Promise.reject())
        .then((categories) => {
            nav.querySelectorAll(".app-navigation__group").forEach((group) => group.remove());
            categories.forEach((category) => {
                const group = document.createElement("div");
                group.className = "app-navigation__group";
                const title = document.createElement("button");
                title.type = "button";
                title.className = "app-navigation__section";
                title.innerHTML = '<span></span><span class="app-navigation__chevron" aria-hidden="true">›</span>';
                title.firstElementChild.textContent = category.Name;
                const submenu = document.createElement("div");
                submenu.className = "app-navigation__submenu";
                let categoryActive = false;
                category.Modules.forEach((module) => {
                    const link = document.createElement("a");
                    link.className = "app-navigation__link";
                    if (currentPath === module.Route || (module.Route !== "/go/dashboard" && currentPath.startsWith(module.Route + "/"))) {
                        link.classList.add("is-active");
                        link.setAttribute("aria-current", "page");
                        categoryActive = true;
                    }
                    link.href = module.Route;
                    link.innerHTML = '<span class="app-navigation__icon" aria-hidden="true"></span><span></span>';
                    link.children[0].textContent = module.Icon;
                    link.children[1].textContent = module.Name;
                    submenu.appendChild(link);
                });
                group.classList.toggle("is-open", categoryActive);
                title.setAttribute("aria-expanded", String(categoryActive));
                submenu.hidden = !categoryActive;
                title.addEventListener("click", () => {
                    const open = !group.classList.contains("is-open");
					if (open) {
						nav.querySelectorAll(".app-navigation__group.is-open").forEach((other) => {
							if (other === group) return;
							other.classList.remove("is-open");
							other.querySelector(".app-navigation__section")?.setAttribute("aria-expanded", "false");
							const otherSubmenu = other.querySelector(".app-navigation__submenu");
							if (otherSubmenu) otherSubmenu.hidden = true;
						});
					}
                    group.classList.toggle("is-open", open);
                    title.setAttribute("aria-expanded", String(open));
                    submenu.hidden = !open;
                });
                group.append(title, submenu);
                nav.appendChild(group);
            });
        })
        .catch(() => {});
    sidebar.append(sidebarClose, nav);

    const backdrop = document.createElement("button");
    backdrop.className = "app-sidebar__backdrop";
    backdrop.type = "button";
    backdrop.setAttribute("aria-label", "Fermer le menu");

    const workspace = document.createElement("div");
    workspace.className = "app-workspace";
    const footer = document.createElement("footer");
    footer.className = "app-footer";
    footer.innerHTML = `<span>AegisAdmin</span><span>Version ${document.querySelector('link[href*="?v="]')?.href.split("?v=")[1] || ""}</span>`;
    workspace.append(main, footer);
    shell.append(header, sidebar, backdrop, workspace);
    document.body.appendChild(shell);

    const time = header.querySelector(".app-header__time");
    const updateTime = () => { time.textContent = new Intl.DateTimeFormat("fr-FR", {dateStyle: "short", timeStyle: "short"}).format(new Date()); };
    updateTime();
    const themeSelector = header.querySelector(".app-theme-selector__select");
    themeSelector.value = activeTheme;
    themeSelector.addEventListener("change", () => {
        document.documentElement.dataset.theme = themeSelector.value;
        document.cookie = `aegisadmin_theme=${themeSelector.value}; Path=/; Max-Age=31536000; Secure; SameSite=Strict`;
    });
    header.querySelector(".app-header__button").addEventListener("click", () => window.location.reload());

    const menuButton = header.querySelector(".app-header__menu-button");
    const setOpen = (open) => {
        document.body.classList.toggle("sidebar-open", open);
        menuButton.setAttribute("aria-expanded", String(open));
    };
    menuButton.addEventListener("click", () => setOpen(!document.body.classList.contains("sidebar-open")));
    sidebarClose.addEventListener("click", () => setOpen(false));
    backdrop.addEventListener("click", () => setOpen(false));
    sidebar.addEventListener("click", (event) => { if (event.target.closest("a")) setOpen(false); });
    document.addEventListener("keydown", (event) => { if (event.key === "Escape") setOpen(false); });

    const scrollToTop = document.createElement("button");
    scrollToTop.className = "scroll-to-top";
    scrollToTop.type = "button";
    scrollToTop.setAttribute("aria-label", "Remonter en haut de la page");
    scrollToTop.title = "Remonter en haut";
    scrollToTop.textContent = "↑";
    document.body.appendChild(scrollToTop);
    const updateScrollToTop = () => scrollToTop.classList.toggle("is-visible", window.scrollY > 500);
    window.addEventListener("scroll", updateScrollToTop, {passive: true});
    scrollToTop.addEventListener("click", () => window.scrollTo({top: 0, behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth"}));
    updateScrollToTop();

    const composer = document.querySelector('[data-composer-refreshing="true"]');
    if (composer) {
        window.setTimeout(() => window.location.reload(), 3000);
    }

    const dashboardResources = document.querySelector("[data-dashboard-resources]");
    if (dashboardResources) {
        const interval = Math.max(1000, Number(dashboardResources.dataset.dashboardResourcesInterval) || 2000);
        let updating = false;
        const refreshResources = async () => {
            if (updating || document.hidden) return;
            updating = true;
            try {
                const response = await fetch(dashboardResources.dataset.dashboardResourcesUrl, {headers: {"Accept": "application/json"}, credentials: "same-origin", cache: "no-store"});
                if (!response.ok) throw new Error();
                const cards = await response.json();
                cards.forEach((card) => {
                    const element = dashboardResources.querySelector(`[data-dashboard-card="${CSS.escape(card.id)}"]`);
                    if (!element) return;
                    element.querySelector("h3").textContent = card.title;
                    element.querySelector(".overview-card__value").textContent = card.value;
                    element.querySelector(".muted").textContent = card.subtitle;
                    element.classList.remove("overview-card--success", "overview-card--warning", "overview-card--danger", "overview-card--neutral");
                    element.classList.add(`overview-card--${["success", "warning", "danger"].includes(card.status) ? card.status : "neutral"}`);
                });
            } catch (_) {
                // Le prochain cycle tentera à nouveau l’actualisation.
            } finally {
                updating = false;
            }
        };
        window.setInterval(refreshResources, interval);
    }

    const dashboardSupervision = document.querySelector("[data-dashboard-supervision]");
    if (dashboardSupervision) {
        const interval = Math.max(5000, Number(dashboardSupervision.dataset.dashboardSupervisionInterval) || 15000);
        let updating = false;
        const refreshSupervision = async () => {
            if (updating || document.hidden) return;
            updating = true;
            try {
                const response = await fetch(dashboardSupervision.dataset.dashboardSupervisionUrl, {headers: {"Accept": "application/json"}, credentials: "same-origin", cache: "no-store"});
                if (!response.ok) throw new Error();
                const cards = await response.json();
                cards.forEach((card) => {
                    const element = dashboardSupervision.querySelector(`[data-dashboard-card="${CSS.escape(card.id)}"]`);
                    if (!element) return;
                    element.querySelector("h3").textContent = card.title;
                    element.querySelector(".overview-card__value").textContent = card.value;
                    element.querySelector(".muted").textContent = card.subtitle;
                    element.classList.remove("overview-card--success", "overview-card--warning", "overview-card--danger", "overview-card--neutral");
                    element.classList.add(`overview-card--${["success", "warning", "danger"].includes(card.status) ? card.status : "neutral"}`);
                });
            } catch (_) {
                // Le prochain cycle tentera à nouveau l’actualisation.
            } finally {
                updating = false;
            }
        };
        window.setInterval(refreshSupervision, interval);
    }

    const dashboardUpdates = document.querySelector("[data-dashboard-updates]");
    if (dashboardUpdates) {
        const value = dashboardUpdates.querySelector("[data-dashboard-updates-value]");
        const link = dashboardUpdates.querySelector("[data-dashboard-updates-link]");
        fetch(dashboardUpdates.dataset.dashboardUpdatesUrl, {headers: {"Accept": "application/json"}, credentials: "same-origin", cache: "no-store"})
            .then(async (response) => {
                const summary = await response.json();
                if (!response.ok || !summary.success) throw new Error(summary.message || "État indisponible");
                const label = summary.statusLabel && summary.statusLabel !== summary.value ? `${summary.value} · ${summary.statusLabel}` : summary.value;
                value.textContent = label;
                value.hidden = Boolean(summary.updatesAvailable);
                link.hidden = !summary.updatesAvailable;
                link.textContent = label;
                dashboardUpdates.title = summary.subtitle || "";
                dashboardUpdates.classList.remove("is-success", "is-warning", "is-danger", "is-neutral");
                dashboardUpdates.classList.add(`is-${["success", "warning", "danger"].includes(summary.status) ? summary.status : "neutral"}`);
                dashboardUpdates.setAttribute("aria-busy", "false");
            })
            .catch(() => {
                value.textContent = "État indisponible";
                link.hidden = true;
                dashboardUpdates.setAttribute("aria-busy", "false");
            });
    }

    document.querySelectorAll("[data-sortable-table]").forEach((table) => {
        const body = table.tBodies[0];
        const buttons = Array.from(table.querySelectorAll("[data-sort-column]"));
        const sortTable = (button, requestedDirection = "") => {
                const column = Number(button.dataset.sortColumn);
                const header = button.closest("th");
                const ascending = requestedDirection
                    ? requestedDirection !== "descending"
                    : header.getAttribute("aria-sort") !== "ascending";
                table.querySelectorAll("thead th[aria-sort]").forEach((item) => item.setAttribute("aria-sort", "none"));
                header.setAttribute("aria-sort", ascending ? "ascending" : "descending");
                const rows = Array.from(body.rows);
                rows.sort((left, right) => {
                    const a = left.cells[column]?.dataset.sortValue || "";
                    const b = right.cells[column]?.dataset.sortValue || "";
                    const comparison = button.dataset.sortType === "number"
                        ? Number(a) - Number(b)
                        : a.localeCompare(b, "fr", {numeric: true, sensitivity: "base"});
                    return ascending ? comparison : -comparison;
                });
                rows.forEach((row) => body.appendChild(row));
        };
        buttons.forEach((button) => {
            button.addEventListener("click", () => sortTable(button));
        });
        const defaultColumn = table.dataset.sortDefaultColumn;
        const defaultButton = buttons.find((button) => button.dataset.sortColumn === defaultColumn);
        if (defaultButton) sortTable(defaultButton, table.dataset.sortDefaultDirection || "ascending");
    });

    const cronDialog = document.querySelector("[data-cron-edit-dialog]");
    document.querySelectorAll("[data-cron-action]").forEach((select) => {
        select.addEventListener("change", () => {
            const action = select.value;
            select.value = "";
            if (!action) return;
            if (action === "edit" && cronDialog) {
                cronDialog.querySelector("[data-cron-edit-user]").value = select.dataset.user;
                cronDialog.querySelector("[data-cron-edit-task-id]").value = select.dataset.taskId;
                cronDialog.querySelector("[data-cron-edit-schedule]").value = select.dataset.schedule;
                cronDialog.querySelector("[data-cron-edit-command]").value = select.dataset.command;
                cronDialog.showModal();
                return;
            }
            if (action === "delete" && !window.confirm("Supprimer cette tâche Cron ?")) return;
            const form = document.createElement("form");
            form.method = "post";
            form.action = `/cron/${action}`;
            [["_token", select.dataset.csrf], ["user", select.dataset.user], ["task_id", select.dataset.taskId]].forEach(([name, value]) => {
                const input = document.createElement("input"); input.type = "hidden"; input.name = name; input.value = value; form.appendChild(input);
            });
            document.body.appendChild(form);
            form.submit();
        });
    });
    cronDialog?.querySelector("[data-cron-edit-close]")?.addEventListener("click", () => cronDialog.close());

    const cronRunDialog = document.querySelector("[data-cron-run-dialog]");
    const cronExecution = new URLSearchParams(window.location.search).get("execution");
    if (cronRunDialog && /^[a-f0-9]{32}$/.test(cronExecution || "")) {
        const status = cronRunDialog.querySelector("[data-cron-run-status]");
        const duration = cronRunDialog.querySelector("[data-cron-run-duration]");
        const exitCode = cronRunDialog.querySelector("[data-cron-run-exit-code]");
        const output = cronRunDialog.querySelector("[data-cron-run-output]");
        const stdout = cronRunDialog.querySelector("[data-cron-run-stdout]");
        const stderr = cronRunDialog.querySelector("[data-cron-run-stderr]");
        const notice = cronRunDialog.querySelector("[data-cron-run-notice]");
        let attempts = 0;
        const loadCronResult = async () => {
            attempts += 1;
            try {
                const response = await fetch(`/cron/executions/${cronExecution}`, {headers: {"Accept": "application/json"}, credentials: "same-origin", cache: "no-store"});
                if (!response.ok) throw new Error(await response.text());
                const result = await response.json();
                const running = result.status === "running";
                status.textContent = running ? "Exécution en cours…" : result.status === "finished" && result.exit_code === 0 ? "Exécution terminée avec succès." : "L’exécution s’est terminée en erreur.";
                duration.textContent = running ? "—" : `${new Intl.NumberFormat("fr-FR").format(result.duration_ms)} ms`;
                exitCode.textContent = result.exit_code === null ? "—" : String(result.exit_code);
                stdout.textContent = result.stdout || "(aucune sortie)";
                stderr.textContent = result.stderr || "(aucune erreur)";
                output.hidden = running;
                notice.hidden = !(result.timed_out || result.truncated);
                notice.textContent = result.timed_out ? "L’exécution a dépassé la durée maximale." : result.truncated ? "La sortie a été tronquée." : "";
                if (running && attempts < 100) window.setTimeout(loadCronResult, 750);
            } catch (error) {
                status.textContent = "Le résultat ne peut pas être chargé.";
                notice.hidden = false; notice.textContent = (error.message || "Erreur inconnue").trim();
            }
        };
        cronRunDialog.showModal();
        loadCronResult();
    }
    cronRunDialog?.querySelector("[data-cron-close-run]")?.addEventListener("click", () => cronRunDialog.close());

    const apacheCreateDialog = document.querySelector("[data-apache-create-dialog]");
    const apacheEditDialog = document.querySelector("[data-apache-edit-dialog]");
    const apacheCertificateDialog = document.querySelector("[data-apache-certificate-dialog]");
    document.querySelector("[data-apache-create-open]")?.addEventListener("click", () => apacheCreateDialog?.showModal());
    apacheCreateDialog?.querySelector("[data-apache-create-close]")?.addEventListener("click", () => apacheCreateDialog.close());
    apacheEditDialog?.querySelector("[data-apache-edit-close]")?.addEventListener("click", () => apacheEditDialog.close());
    apacheCertificateDialog?.querySelector("[data-apache-certificate-close]")?.addEventListener("click", () => apacheCertificateDialog.close());
    const apacheSiteType = apacheCreateDialog?.querySelector("[data-apache-site-type]");
    const updateApacheSiteType = () => {
        const proxy = apacheSiteType?.value === "proxy";
        const websiteFields = apacheCreateDialog?.querySelector("[data-apache-website-fields]");
        const proxyFields = apacheCreateDialog?.querySelector("[data-apache-proxy-fields]");
        if (websiteFields) websiteFields.hidden = proxy;
        if (proxyFields) proxyFields.hidden = !proxy;
        websiteFields?.querySelectorAll("input[required]").forEach((input) => { input.disabled = proxy; });
        proxyFields?.querySelectorAll("input").forEach((input) => { input.required = proxy; input.disabled = !proxy; });
    };
    apacheSiteType?.addEventListener("change", updateApacheSiteType);
    updateApacheSiteType();
    document.querySelectorAll("[data-apache-site-action]").forEach((select) => {
        select.addEventListener("change", async () => {
            const action = select.value; select.value = ""; if (!action) return;
            if (action === "edit" && apacheEditDialog) {
                try {
                    const response = await fetch(`/apache/sites/${encodeURIComponent(select.dataset.configId)}`, {headers: {"Accept": "application/json"}, credentials: "same-origin", cache: "no-store"});
                    if (!response.ok) throw new Error(await response.text());
                    const config = await response.json();
                    apacheEditDialog.querySelector("[name=config_id]").value = config.config_id;
                    apacheEditDialog.querySelector("[name=content]").value = config.content;
                    apacheEditDialog.querySelector("[data-apache-edit-filename]").textContent = config.filename;
                    apacheEditDialog.showModal();
                } catch (error) { window.alert(error.message || "La configuration ne peut pas être chargée."); }
                return;
            }
            if (action === "certificate" && apacheCertificateDialog) {
                apacheCertificateDialog.querySelector("[name=domains]").value = select.dataset.domains;
                apacheCertificateDialog.showModal(); return;
            }
            if (action === "delete" && !window.confirm(`Supprimer définitivement ${select.dataset.filename} ?`)) return;
            const form = document.createElement("form"); form.method = "post"; form.action = `/apache/${action}`;
            [["_token", select.dataset.csrf], ["config_id", select.dataset.configId]].forEach(([name, value]) => { const input = document.createElement("input"); input.type = "hidden"; input.name = name; input.value = value; form.appendChild(input); });
            document.body.appendChild(form); form.submit();
        });
    });

    document.querySelectorAll("[data-certbot-certificate-action]").forEach((select) => {
        select.addEventListener("change", () => {
            const action = select.value; select.value = ""; if (!action) return;
            const certificate = select.dataset.certificate || "";
            const labels = {reinstall: "Réinstaller", "renew-replace": "Renouveler", delete: "Supprimer"};
            if (!window.confirm(`${labels[action] || "Exécuter l’action sur"} le certificat ${certificate} ?`)) return;
            const form = document.createElement("form"); form.method = "post"; form.action = `/certbot/${action}`;
            [["_token", select.dataset.csrf], ["certificate", certificate]].forEach(([name, value]) => { const input = document.createElement("input"); input.type = "hidden"; input.name = name; input.value = value; form.appendChild(input); });
            document.body.appendChild(form); form.submit();
        });
    });

    document.querySelectorAll("[data-firewall-state-form]").forEach((form) => {
        form.addEventListener("submit", (event) => {
            const disabling = form.dataset.firewallState === "disable";
            const message = disabling
                ? "Désactiver le pare-feu peut exposer le serveur. Confirmer la désactivation ?"
                : "Confirmer l’activation du pare-feu ? Vérifiez qu’une règle autorise votre accès d’administration.";
            if (!window.confirm(message)) event.preventDefault();
        });
    });

    const mysqlMetrics = document.querySelector("[data-mysql-metrics]");
    if (mysqlMetrics) {
        const interval = Math.max(1000, Number(mysqlMetrics.dataset.mysqlMetricsInterval) || 2000);
        let updating = false;
        const refreshMySQLMetrics = async () => {
            if (updating || document.hidden) return;
            updating = true;
            try {
                const response = await fetch(mysqlMetrics.dataset.mysqlMetricsUrl, {headers: {"Accept": "application/json"}, credentials: "same-origin", cache: "no-store"});
                if (!response.ok) throw new Error();
                const metrics = await response.json();
                Object.entries(metrics).forEach(([key, value]) => {
                    const target = mysqlMetrics.querySelector(`[data-mysql-metric="${CSS.escape(key)}"] strong`);
                    if (target) target.textContent = new Intl.NumberFormat("fr-FR").format(value);
                });
            } catch (_) {
                // Les dernières valeurs valides restent visibles.
            } finally {
                updating = false;
            }
        };
        window.setInterval(refreshMySQLMetrics, interval);
    }

    const composerModal = document.querySelector("[data-composer-modal]");
    if (composerModal) {
        const modalBody = composerModal.querySelector("[data-composer-modal-body]");
        const closeModal = () => {
            composerModal.hidden = true;
            modalBody.replaceChildren();
            document.body.classList.remove("modal-open");
        };
        document.addEventListener("click", (event) => {
            const opener = event.target.closest("[data-composer-detail-open]");
            if (opener) {
                const template = document.getElementById(opener.dataset.composerDetailOpen || "");
                if (template instanceof HTMLTemplateElement) {
                    modalBody.replaceChildren(template.content.cloneNode(true));
                    composerModal.hidden = false;
                    document.body.classList.add("modal-open");
                    composerModal.querySelector(".modal__close").focus();
                }
            }
            if (event.target.closest("[data-modal-close]")) closeModal();
        });
        document.addEventListener("keydown", (event) => { if (event.key === "Escape" && !composerModal.hidden) closeModal(); });
    }

    const updateJobModal = document.querySelector("[data-update-job-modal]");
    const updateJobID = new URLSearchParams(window.location.search).get("job");
    if (updateJobModal && /^[a-f0-9]{32}$/.test(updateJobID || "")) {
        const status = updateJobModal.querySelector("[data-update-job-status]");
        const state = updateJobModal.querySelector("[data-update-job-state]");
        const backend = updateJobModal.querySelector("[data-update-job-backend]");
        const output = updateJobModal.querySelector("[data-update-job-output]");
        const started = updateJobModal.querySelector("[data-update-job-started]");
        const finished = updateJobModal.querySelector("[data-update-job-finished]");
        const exitCode = updateJobModal.querySelector("[data-update-job-exit-code]");
        const notice = updateJobModal.querySelector("[data-update-job-notice]");
        const closeButtons = updateJobModal.querySelectorAll("[data-update-job-close]");
        let complete = false;
        let pollingTimer;

        const formatDate = (value) => {
            if (!value) return "—";
            const date = new Date(value);
            return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat("fr-FR", {dateStyle: "short", timeStyle: "medium"}).format(date);
        };
        const close = () => {
            if (!complete) return;
            window.clearTimeout(pollingTimer);
            updateJobModal.hidden = true;
            document.body.classList.remove("modal-open");
            const location = new URL(window.location.href);
            location.searchParams.delete("job");
            window.history.replaceState({}, "", location);
        };
        const finish = (job) => {
            complete = true;
            closeButtons.forEach((button) => { button.disabled = false; });
            const succeeded = job.status === "completed" && job.exit_code === 0;
            status.textContent = succeeded ? "Mise à jour terminée avec succès." : "La mise à jour s’est terminée avec une erreur.";
            state.textContent = succeeded ? "Terminée" : "Échec";
            updateJobModal.dataset.updateJobResult = succeeded ? "success" : "failed";
            notice.hidden = succeeded;
            if (!succeeded) notice.textContent = "Consultez les dernières lignes du terminal pour identifier la cause de l’échec.";
        };
        const poll = async () => {
            try {
                const response = await fetch(`/updates/jobs/${encodeURIComponent(updateJobID)}`, {headers: {"Accept": "application/json"}, credentials: "same-origin", cache: "no-store"});
                if (!response.ok) throw new Error("status");
                const job = await response.json();
                const lines = Array.isArray(job.lines) ? job.lines : [];
                backend.textContent = String(job.backend || "Gestionnaire de paquets").toUpperCase();
                output.textContent = lines.length ? lines.join("\n") : "En attente de la première sortie…";
                output.scrollTop = output.scrollHeight;
                started.textContent = formatDate(job.started_at);
                finished.textContent = formatDate(job.finished_at);
                exitCode.textContent = job.exit_code === null || job.exit_code === undefined ? "—" : String(job.exit_code);
                if (job.status === "running") {
                    status.textContent = "Installation en cours… Cette fenêtre se fermera uniquement à la fin.";
                    state.textContent = "En cours";
                    updateJobModal.dataset.updateJobResult = "running";
                    notice.hidden = true;
                    pollingTimer = window.setTimeout(poll, 1000);
                    return;
                }
                finish(job);
            } catch (_) {
                status.textContent = "Connexion au suivi momentanément indisponible.";
                state.textContent = "Reconnexion…";
                notice.textContent = "Le travail continue sur le serveur. Nouvelle tentative dans deux secondes.";
                notice.hidden = false;
                pollingTimer = window.setTimeout(poll, 2000);
            }
        };

        closeButtons.forEach((button) => button.addEventListener("click", close));
        document.addEventListener("keydown", (event) => { if (event.key === "Escape") close(); });
        updateJobModal.hidden = false;
        document.body.classList.add("modal-open");
        poll();
    }

    document.querySelectorAll("[data-password-generator]").forEach((form) => {
        const password = form.querySelector("[data-generated-password]");
        const confirmation = form.querySelector("[data-generated-password-confirmation]");
        const generate = form.querySelector("[data-generate-password]");
        const toggle = form.querySelector("[data-toggle-password]");
        const copy = form.querySelector("[data-copy-password]");
        const status = form.querySelector("[data-password-status]");
        if (!password || !confirmation || !generate || !toggle || !copy || !status) return;

        const randomIndex = (length) => {
            const maximum = Math.floor(256 / length) * length;
            const value = new Uint8Array(1);
            do { window.crypto.getRandomValues(value); } while (value[0] >= maximum);
            return value[0] % length;
        };
        const pick = (characters) => characters[randomIndex(characters.length)];
        const shuffle = (characters) => {
            for (let index = characters.length - 1; index > 0; index--) {
                const swap = randomIndex(index + 1);
                [characters[index], characters[swap]] = [characters[swap], characters[index]];
            }
            return characters.join("");
        };
        const createPassword = () => {
            const groups = ["abcdefghijkmnopqrstuvwxyz", "ABCDEFGHJKLMNPQRSTUVWXYZ", "23456789", "!@#$%*-_=+?"];
            const all = groups.join("");
            const characters = groups.map(pick);
            while (characters.length < 24) characters.push(pick(all));
            return shuffle(characters);
        };
        const updateCopyState = () => { copy.disabled = password.value === ""; };

        generate.addEventListener("click", () => {
            const value = createPassword();
            password.value = value;
            confirmation.value = value;
            updateCopyState();
            status.textContent = "Mot de passe aléatoire de 24 caractères généré et confirmé.";
            password.focus();
        });
        toggle.addEventListener("click", () => {
            const visible = password.type === "text";
            password.type = visible ? "password" : "text";
            confirmation.type = visible ? "password" : "text";
            toggle.textContent = visible ? "Afficher" : "Masquer";
        });
        copy.addEventListener("click", async () => {
            try {
                await navigator.clipboard.writeText(password.value);
                status.textContent = "Mot de passe copié dans le presse-papiers.";
            } catch (_) {
                password.type = "text";
                confirmation.type = "text";
                password.select();
                toggle.textContent = "Masquer";
                status.textContent = "Copie automatique impossible : le mot de passe est sélectionné.";
            }
        });
        password.addEventListener("input", updateCopyState);
    });

    const navigationAdmin = document.querySelector("[data-navigation-admin]");
    if (navigationAdmin) {
        navigationAdmin.classList.add("drag-enabled");
        const categories = navigationAdmin.querySelector("[data-navigation-categories]");
        const status = navigationAdmin.querySelector("[data-navigation-status]");
        let draggedCategory = null;
        let draggedModule = null;
        let armedModule = null;
        let initialOrder = "";

        const serializeOrder = () => {
            const categoryIds = [];
            const moduleIdsByCategory = {};
            categories.querySelectorAll(":scope > [data-navigation-category]").forEach((category) => {
                const categoryId = Number(category.dataset.categoryId);
                categoryIds.push(categoryId);
                moduleIdsByCategory[categoryId] = Array.from(category.querySelectorAll("[data-navigation-module-list] > [data-navigation-module]")).map((module) => Number(module.dataset.moduleId));
            });
            return {categoryIds, moduleIdsByCategory};
        };
        const updateEmptyRows = () => {
            navigationAdmin.querySelectorAll("[data-navigation-module-list]").forEach((list) => {
                const empty = list.querySelector("[data-navigation-empty-row]");
                const hasModules = list.querySelector("[data-navigation-module]");
                if (hasModules && empty) empty.remove();
                if (!hasModules && !empty) {
                    const row = document.createElement("tr");
                    row.dataset.navigationEmptyRow = "";
                    row.innerHTML = '<td class="muted" colspan="5">Déposez un module dans cette catégorie.</td>';
                    list.appendChild(row);
                }
            });
        };
        const showNavigationStatus = (message, success) => {
            status.hidden = false;
            status.textContent = message;
            status.className = `notice ${success ? "notice--success" : "error"}`;
        };
        const persistOrder = async () => {
            const order = serializeOrder();
            if (JSON.stringify(order) === initialOrder) return;
            showNavigationStatus("Enregistrement de l’organisation…", true);
            try {
                const response = await fetch(navigationAdmin.dataset.navigationReorderUrl, {method: "POST", credentials: "same-origin", headers: {"Accept": "application/json", "Content-Type": "application/json"}, body: JSON.stringify({_token: navigationAdmin.dataset.navigationCsrf, ...order})});
                const result = await response.json();
                if (!response.ok || result.success !== true) throw new Error(result.message || "Enregistrement impossible.");
                showNavigationStatus(result.message, true);
                window.setTimeout(() => window.location.reload(), 500);
            } catch (error) {
                showNavigationStatus(error instanceof Error ? error.message : "Enregistrement impossible.", false);
                window.setTimeout(() => window.location.reload(), 1200);
            }
        };
        navigationAdmin.addEventListener("pointerdown", (event) => { armedModule = event.target.closest("[data-navigation-module-handle]")?.closest("[data-navigation-module]") || null; });
        navigationAdmin.addEventListener("dragstart", (event) => {
            const categoryHandle = event.target.closest("[data-navigation-category-handle]");
            if (categoryHandle) {
                draggedCategory = categoryHandle.closest("[data-navigation-category]");
                initialOrder = JSON.stringify(serializeOrder());
                draggedCategory.classList.add("is-dragging");
                return;
            }
            const module = event.target.closest("[data-navigation-module]");
            if (!module || module !== armedModule) { event.preventDefault(); return; }
            draggedModule = module;
            initialOrder = JSON.stringify(serializeOrder());
            draggedModule.classList.add("is-dragging");
        });
        navigationAdmin.addEventListener("dragover", (event) => {
            if (draggedCategory) {
                const target = event.target.closest("[data-navigation-category]");
                if (!target || target === draggedCategory) return;
                event.preventDefault();
                categories.insertBefore(draggedCategory, event.clientY > target.getBoundingClientRect().top + target.offsetHeight / 2 ? target.nextElementSibling : target);
            } else if (draggedModule) {
                const list = event.target.closest("[data-navigation-module-list]");
                if (!list) return;
                event.preventDefault();
                const target = event.target.closest("[data-navigation-module]");
                if (!target || target === draggedModule || target.parentElement !== list) list.appendChild(draggedModule);
                else list.insertBefore(draggedModule, event.clientY > target.getBoundingClientRect().top + target.offsetHeight / 2 ? target.nextElementSibling : target);
                updateEmptyRows();
            }
        });
        navigationAdmin.addEventListener("drop", (event) => event.preventDefault());
        navigationAdmin.addEventListener("dragend", async () => {
            draggedCategory?.classList.remove("is-dragging");
            draggedModule?.classList.remove("is-dragging");
            draggedCategory = null; draggedModule = null; armedModule = null;
            updateEmptyRows();
            await persistOrder();
        });
    }
}());
