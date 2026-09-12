(function () {
    "use strict";

    if (!document.body.classList.contains("dashboard-body")) {
        return;
    }

    const languageCookie = document.cookie.match(/(?:^|; )aegisadmin_language=([^;]+)/)?.[1];
    let activeLanguage = ["fr", "en"].includes(languageCookie) ? languageCookie : (document.documentElement.lang || "fr");
    const words = {
        fr: {theme: "Thème", refresh: "Actualiser", menu: "Menu", openMenu: "Ouvrir le menu", closeMenu: "Fermer le menu", account: "Mon compte", saveError: "La préférence n’a pas pu être enregistrée dans votre profil.", apacheLoadError: "La configuration ne peut pas être chargée.", apacheDelete: "Supprimer définitivement", dashboardStateUnavailable: "État indisponible"},
        en: {theme: "Theme", refresh: "Refresh", menu: "Menu", openMenu: "Open menu", closeMenu: "Close menu", account: "My account", saveError: "The preference could not be saved to your profile.", apacheLoadError: "The configuration could not be loaded.", apacheDelete: "Permanently delete", dashboardStateUnavailable: "Status unavailable"}
    };
    const tr = (key) => words[activeLanguage]?.[key] || words.fr[key] || key;

    const routesByLanguage = {
        fr: [["Vue générale", [["⌂", "Tableau de bord", "/go/dashboard"]]], ["Supervision", [["◫", "Stockage", "/storage"], ["●", "Services", "/services"], ["↔", "Réseau", "/network"], ["≡", "Journaux", "/logs"]]], ["Services", [["◆", "Apache", "/apache"], ["PHP", "PHP", "/php"], ["DB", "MySQL", "/mysql"], ["◉", "Tor", "/tor"], ["▦", "Fail2ban", "/fail2ban"], ["▤", "Pare-feu", "/firewall"], ["◷", "Cron", "/cron"], ["◇", "Certificats TLS", "/certbot"]]], ["Administration", [["↑", "Mises à jour", "/updates"], ["♙", "Utilisateurs", "/users"], ["▦", "Modules", "/modules"], ["⚙", "Paramètres", "/setting"], ["≣", "Config. serveur", "/configuration"], ["i", "À propos", "/about"]]]],
        en: [["Overview", [["⌂", "Dashboard", "/go/dashboard"]]], ["Monitoring", [["◫", "Storage", "/storage"], ["●", "Services", "/services"], ["↔", "Network", "/network"], ["≡", "Logs", "/logs"]]], ["Services", [["◆", "Apache", "/apache"], ["PHP", "PHP", "/php"], ["DB", "MySQL", "/mysql"], ["◉", "Tor", "/tor"], ["▦", "Fail2ban", "/fail2ban"], ["▤", "Firewall", "/firewall"], ["◷", "Cron", "/cron"], ["◇", "TLS certificates", "/certbot"]]], ["Administration", [["↑", "Updates", "/updates"], ["♙", "Users", "/users"], ["▦", "Modules", "/modules"], ["⚙", "Settings", "/setting"], ["≣", "Server config.", "/configuration"], ["i", "About", "/about"]]]]
    };
    const routes = routesByLanguage[activeLanguage] || routesByLanguage.fr;
    const translatedRoutes = new Map(routes.flatMap(([, links]) => links.map(([, label, route]) => [route, label])));
    const translatedCategories = new Map(routesByLanguage.fr.map(([name], index) => [name, routes[index]?.[0] || name]));
    translatedCategories.set("Sécurité", activeLanguage === "en" ? "Security" : "Sécurité");

    const main = document.querySelector("main.dashboard-shell");
    if (!main) return;

    const currentPath = window.location.pathname.replace(/\/$/, "") || "/";
    const themeStylesheet = document.createElement("link");
    themeStylesheet.rel = "stylesheet";
    themeStylesheet.href = `/assets/themes.css?v=${document.querySelector('link[href*="?v="]')?.href.split("?v=")[1] || ""}`;
    document.head.appendChild(themeStylesheet);
    const storedTheme = document.cookie.match(/(?:^|; )aegisadmin_theme=([^;]+)/)?.[1];
    let activeTheme = ["dark", "light", "bootstrap", "neon"].includes(storedTheme) ? storedTheme : "dark";
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
        <button class="app-header__menu-button" type="button" aria-label="${tr("openMenu")}" aria-expanded="false"><span>☰</span><span>${tr("menu")}</span></button>
        <div class="app-header__actions">
            <label class="app-theme-selector"><span class="app-theme-selector__label">${tr("theme")}</span><select class="app-theme-selector__select" aria-label="${tr("theme")}"><option value="dark">Sombre</option><option value="light">Clair</option><option value="bootstrap">Bootstrap</option><option value="neon">Néon Pop</option></select></label>
            <span class="app-header__time"></span><button class="app-header__button" type="button">${tr("refresh")}</button>
        </div>`;

    const sidebar = document.createElement("aside");
    sidebar.className = "app-sidebar";
    sidebar.id = "app-sidebar";
    sidebar.setAttribute("aria-label", "Menu principal");
    const sidebarClose = document.createElement("button");
    sidebarClose.className = "app-sidebar__close";
    sidebarClose.type = "button";
    sidebarClose.setAttribute("aria-label", tr("closeMenu"));
    sidebarClose.textContent = `× ${tr("closeMenu")}`;
    const nav = document.createElement("nav");
    nav.className = "app-navigation";
    routes.forEach(([category, links], categoryIndex) => {
        const group = document.createElement("div");
        group.className = "app-navigation__group";
        group.dataset.accent = String(categoryIndex % 6);
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
    userMenu.innerHTML = `<a class="app-user-menu__login" href="/go/account/password">${tr("account")}</a>`;
    const logout = main.querySelector('form[action="/logout"]');
    const csrfToken = logout?.querySelector('input[name="_token"]')?.value || "";
    if (logout) {
        logout.querySelector("button")?.classList.add("button", "button--neutral", "button--sm");
        userMenu.appendChild(logout);
    }
    nav.prepend(userMenu);
    fetch("/go/navigation", {headers: {"Accept": "application/json"}, credentials: "same-origin"})
        .then((response) => response.ok ? response.json() : Promise.reject())
        .then((categories) => {
            nav.querySelectorAll(".app-navigation__group").forEach((group) => group.remove());
            categories.forEach((category, categoryIndex) => {
                const group = document.createElement("div");
                group.className = "app-navigation__group";
                group.dataset.accent = String(categoryIndex % 6);
                const title = document.createElement("button");
                title.type = "button";
                title.className = "app-navigation__section";
                title.innerHTML = '<span></span><span class="app-navigation__chevron" aria-hidden="true">›</span>';
                title.firstElementChild.textContent = translatedCategories.get(category.Name) || category.Name;
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
                    link.children[1].textContent = translatedRoutes.get(module.Route) || module.Name;
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
    const updateTime = () => { time.textContent = new Intl.DateTimeFormat(activeLanguage === "en" ? "en-US" : "fr-FR", {dateStyle: "short", timeStyle: "short"}).format(new Date()); };
    updateTime();
    const themeSelector = header.querySelector(".app-theme-selector__select");
    themeSelector.value = activeTheme;
    themeSelector.addEventListener("change", async () => {
        const selectedTheme = themeSelector.value;
        const previousTheme = activeTheme;
        themeSelector.disabled = true;
        document.documentElement.dataset.theme = selectedTheme;
        document.cookie = `aegisadmin_theme=${selectedTheme}; Path=/; Max-Age=31536000; Secure; SameSite=Strict`;
        try {
            const body = new URLSearchParams({_token: csrfToken, theme: selectedTheme});
            const response = await fetch("/go/account/theme", {method: "POST", headers: {"Content-Type": "application/x-www-form-urlencoded", "Accept": "application/json"}, body, credentials: "same-origin"});
            if (!response.ok) throw new Error();
            activeTheme = selectedTheme;
        } catch (_) {
            activeTheme = previousTheme;
            themeSelector.value = previousTheme;
            document.documentElement.dataset.theme = previousTheme;
            document.cookie = `aegisadmin_theme=${previousTheme}; Path=/; Max-Age=31536000; Secure; SameSite=Strict`;
            window.alert(tr("saveError"));
        } finally {
            themeSelector.disabled = false;
        }
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

    const dashboardUptime = document.querySelector("[data-dashboard-uptime]");
    if (dashboardUptime) {
        let seconds=Number(dashboardUptime.dataset.dashboardUptimeSeconds)||0;
        const formatUptime=()=>{const days=Math.floor(seconds/86400),hours=Math.floor((seconds%86400)/3600),minutes=Math.floor((seconds%3600)/60);const parts=[];if(days)parts.push(`${days} ${activeLanguage === "en" ? "d" : "j"}`);if(hours)parts.push(`${hours} h`);parts.push(`${minutes} min`);dashboardUptime.textContent=parts.join(" ");};
        window.setInterval(()=>{seconds+=60;formatUptime();},60000);
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
                if (!response.ok || !summary.success) throw new Error(summary.message || tr("dashboardStateUnavailable"));
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
                value.textContent = tr("dashboardStateUnavailable");
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

    const cronDialog = document.querySelector("[data-cron-create-dialog]");
    const cronForm = cronDialog?.querySelector("[data-cron-create-form]");
    const cronText = activeLanguage === "en" ? {incomplete:"Incomplete expression",edit:"Edit Cron task",create:"Create a user task",save:"Save",submit:"Create task",delete:"Delete this Cron task?",backupDelete:"Delete this backup task?",running:"Execution in progress…",success:"Execution completed successfully.",failed:"Execution failed.",emptyOut:"(no output)",emptyErr:"(no error)",timeout:"The execution exceeded the maximum duration.",truncated:"The output was truncated.",load:"The result could not be loaded.",unknown:"Unknown error",mysql:["MySQL / MariaDB backup","all","Database backup"],apache:["Apache configuration backup","/etc/apache2","Apache configuration"],sites:["Website backup","/var/www","Websites"]} : {incomplete:"Expression incomplète",edit:"Modifier la tâche Cron",create:"Créer une tâche utilisateur",save:"Enregistrer",submit:"Créer la tâche",delete:"Supprimer cette tâche Cron ?",backupDelete:"Supprimer cette tâche de sauvegarde ?",running:"Exécution en cours…",success:"Exécution terminée avec succès.",failed:"L’exécution s’est terminée en erreur.",emptyOut:"(aucune sortie)",emptyErr:"(aucune erreur)",timeout:"L’exécution a dépassé la durée maximale.",truncated:"La sortie a été tronquée.",load:"Le résultat ne peut pas être chargé.",unknown:"Erreur inconnue",mysql:["Sauvegarde MySQL / MariaDB","all","Sauvegarde des bases de données"],apache:["Sauvegarde de la configuration Apache","/etc/apache2","Configuration Apache"],sites:["Sauvegarde des sites web","/var/www","Sites web"]};
    const cronParts = ["minute","hour","monthday","month","weekday"];
    const selectedCronValues = (part) => Array.from(cronForm?.querySelectorAll(`[data-cron-part="${part}"]:checked`) || []).map((field)=>Number(field.value)).sort((a,b)=>a-b);
    const cronPartExpression = (part, total) => { const values=selectedCronValues(part); return values.length===0||values.length===total?"*":values.join(","); };
    const updateCronSchedule = () => {
        if (!cronForm) return;
        const custom=cronForm.querySelector("[data-cron-mode]").value==="custom";
        cronForm.querySelector("[data-cron-visual]").hidden=custom;
        cronForm.querySelector("[data-cron-custom-field]").hidden=!custom;
        const expression=custom?cronForm.querySelector("[data-cron-custom]").value.trim():[cronPartExpression("minute",60),cronPartExpression("hour",24),cronPartExpression("monthday",31),cronPartExpression("month",12),cronPartExpression("weekday",7)].join(" ");
        cronForm.querySelector("[data-cron-schedule]").value=expression;
        cronForm.querySelector("[data-cron-schedule-preview]").textContent=expression||cronText.incomplete;
        cronForm.querySelector("[data-cron-day-warning]").hidden=custom||selectedCronValues("monthday").length===0||selectedCronValues("weekday").length===0;
    };
    const setCronExpression = (expression) => {
        if (!cronForm) return;
        cronForm.querySelectorAll("[data-cron-part]").forEach((field)=>{field.checked=false;});
        const fields=expression.trim().split(/\s+/), limits=[60,24,31,12,7]; let visual=fields.length===5;
        fields.forEach((field,index)=>{if(!visual)return;if(field==="*")return;if(!/^\d+(?:,\d+)*$/.test(field)){visual=false;return;}const values=field.split(",").map(Number);if(values.some((value)=>value<(index===2||index===3?1:0)||value>(index===0?59:index===1?23:index===2?31:index===3?12:6))){visual=false;return;}values.forEach((value)=>{const choice=cronForm.querySelector(`[data-cron-part="${cronParts[index]}"][value="${value}"]`);if(choice)choice.checked=true;});});
        cronForm.querySelector("[data-cron-mode]").value=visual?"visual":"custom";
        cronForm.querySelector("[data-cron-custom]").value=expression;
        updateCronSchedule();
    };
    const openCronForm = (mode, data={}) => {
        if (!cronDialog||!cronForm) return;
        const editing=mode==="edit"; cronForm.action=editing?"/cron/update":"/cron/create";
        cronForm.querySelector("[data-cron-form-title]").textContent=editing?cronText.edit:cronText.create;
        cronForm.querySelector("[data-cron-submit]").textContent=editing?cronText.save:cronText.submit;
        cronForm.querySelector("[data-cron-task-id]").value=data.taskId||"";
        cronForm.querySelector("[data-cron-user]").value=data.user||cronForm.querySelector("[data-cron-user]").options[0]?.value||"";
        cronForm.querySelector("[data-cron-user]").disabled=editing;
        let hiddenUser=cronForm.querySelector('[data-cron-edit-user-hidden]');if(hiddenUser)hiddenUser.remove();if(editing){hiddenUser=document.createElement("input");hiddenUser.type="hidden";hiddenUser.name="user";hiddenUser.value=data.user;hiddenUser.dataset.cronEditUserHidden="";cronForm.appendChild(hiddenUser);}
        cronForm.querySelector("[data-cron-command]").value=data.command||"";
        setCronExpression(data.schedule||"0 2 * * *"); cronDialog.showModal();
    };
    document.querySelectorAll("[data-cron-action]").forEach((select) => {
        select.addEventListener("change", () => {
            const action = select.value;
            select.value = "";
            if (!action) return;
            if (action === "edit" && cronDialog) {
                openCronForm("edit",select.dataset);
                return;
            }
            if (action === "delete" && !window.confirm(cronText.delete)) return;
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
    cronForm?.querySelectorAll("select,input").forEach((field)=>field.addEventListener("input",updateCronSchedule));
    cronForm?.querySelectorAll("[data-cron-clear]").forEach((button)=>button.addEventListener("click",()=>{cronForm.querySelectorAll(`[data-cron-part="${button.dataset.cronClear}"]`).forEach((field)=>{field.checked=false;});updateCronSchedule();}));
    document.querySelector("[data-cron-create-open]")?.addEventListener("click",()=>openCronForm("create"));
    cronDialog?.querySelector("[data-cron-create-close]")?.addEventListener("click",()=>cronDialog.close());
    cronForm?.addEventListener("submit",(event)=>{updateCronSchedule();if(!cronForm.querySelector("[data-cron-schedule]").value){event.preventDefault();}});

    const backupDialog = document.querySelector("[data-backup-dialog]");
    const backupDefaults = {mysql:cronText.mysql,apache:cronText.apache,sites:cronText.sites};
    document.querySelectorAll("[data-backup-template-select]").forEach((select) => select.addEventListener("change", () => {
        if (!backupDialog) return;
        const kind=select.value, values=backupDefaults[kind]; if (!values) return;
        select.value="";
        backupDialog.querySelector("[data-backup-kind]").value=kind;
        backupDialog.querySelector("[data-backup-title]").textContent=values[0];
        backupDialog.querySelector("[data-backup-source]").value=values[1];
        backupDialog.querySelector('input[name="name"]').value=values[2];
        backupDialog.showModal();
    }));
    backupDialog?.querySelector("[data-backup-close]")?.addEventListener("click", () => backupDialog.close());
    document.querySelectorAll("[data-backup-action]").forEach((select) => select.addEventListener("change", () => {
        const action=select.value; select.value=""; if (!action) return;
        if (action==="delete" && !window.confirm(cronText.backupDelete)) return;
        const form=document.createElement("form"); form.method="post"; form.action=`/cron/backup/${action}`;
        [["_token",select.dataset.csrf],["task_id",select.dataset.taskId]].forEach(([name,value])=>{const input=document.createElement("input");input.type="hidden";input.name=name;input.value=value;form.appendChild(input);}); document.body.appendChild(form); form.submit();
    }));

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
                status.textContent = running ? cronText.running : result.status === "finished" && result.exit_code === 0 ? cronText.success : cronText.failed;
                duration.textContent = running ? "—" : `${new Intl.NumberFormat(activeLanguage === "en" ? "en-US" : "fr-FR").format(result.duration_ms)} ms`;
                exitCode.textContent = result.exit_code === null ? "—" : String(result.exit_code);
                stdout.textContent = result.stdout || cronText.emptyOut;
                stderr.textContent = result.stderr || cronText.emptyErr;
                output.hidden = running;
                notice.hidden = !(result.timed_out || result.truncated);
                notice.textContent = result.timed_out ? cronText.timeout : result.truncated ? cronText.truncated : "";
                if (running && attempts < 100) window.setTimeout(loadCronResult, 750);
            } catch (error) {
                status.textContent = cronText.load;
                notice.hidden = false; notice.textContent = (error.message || cronText.unknown).trim();
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
                } catch (error) { window.alert(error.message || tr("apacheLoadError")); }
                return;
            }
            if (action === "certificate" && apacheCertificateDialog) {
                apacheCertificateDialog.querySelector("[name=domains]").value = select.dataset.domains;
                apacheCertificateDialog.showModal(); return;
            }
            if (action === "delete" && !window.confirm(`${tr("apacheDelete")} ${select.dataset.filename}?`)) return;
            const form = document.createElement("form"); form.method = "post"; form.action = `/apache/${action}`;
            [["_token", select.dataset.csrf], ["config_id", select.dataset.configId]].forEach(([name, value]) => { const input = document.createElement("input"); input.type = "hidden"; input.name = name; input.value = value; form.appendChild(input); });
            document.body.appendChild(form); form.submit();
        });
    });

    const certbotResultModal = document.querySelector("[data-certbot-result-modal]");
    if (certbotResultModal) {
        const certText = activeLanguage === "en" ? {actions:{issue:"Certificate creation",renew:"Renewal","renew-test":"Renewal test",reinstall:"Reinstallation","renew-replace":"Certificate renewal",delete:"Certificate deletion"},loadError:"The Certbot result could not be loaded.",running:"Running",success:"Successful",failed:"Failed",stillRunning:"The action is still running…",done:"The Certbot action completed successfully.",doneError:"The Certbot action failed.",finished:"Completed",out:"OUTPUT",err:"ERRORS",waiting:"Waiting for Certbot output…",empty:"No additional output.",timeout:"The action exceeded the maximum allowed duration.",truncated:"The output was truncated to preserve the interface.",unavailable:"The result is temporarily unavailable.",error:"Error",unknown:"Unknown error.",loading:"Loading…",loadingResult:"Loading result…"} : {actions:{issue:"Création du certificat",renew:"Renouvellement","renew-test":"Test du renouvellement",reinstall:"Réinstallation","renew-replace":"Renouvellement du certificat",delete:"Suppression du certificat"},loadError:"Le résultat Certbot ne peut pas être chargé.",running:"En cours",success:"Réussie",failed:"Échec",stillRunning:"L’action est toujours en cours…",done:"L’action Certbot s’est terminée avec succès.",doneError:"L’action Certbot s’est terminée en erreur.",finished:"Terminé",out:"SORTIE",err:"ERREURS",waiting:"En attente de la sortie Certbot…",empty:"Aucune sortie supplémentaire.",timeout:"L’action a dépassé la durée maximale autorisée.",truncated:"La sortie a été tronquée pour préserver l’interface.",unavailable:"Le résultat est momentanément indisponible.",error:"Erreur",unknown:"Erreur inconnue.",loading:"Chargement…",loadingResult:"Chargement du résultat…"};
        const status = certbotResultModal.querySelector("[data-certbot-result-status]");
        const action = certbotResultModal.querySelector("[data-certbot-result-action]");
        const state = certbotResultModal.querySelector("[data-certbot-result-state]");
        const duration = certbotResultModal.querySelector("[data-certbot-result-duration]");
        const exitCode = certbotResultModal.querySelector("[data-certbot-result-exit]");
        const terminalState = certbotResultModal.querySelector("[data-certbot-result-terminal-state]");
        const output = certbotResultModal.querySelector("[data-certbot-result-output]");
        const notice = certbotResultModal.querySelector("[data-certbot-result-notice]");
        const actionLabels = certText.actions;
        let pollingTimer;
        let complete = false;
        const close = () => {
            window.clearTimeout(pollingTimer);
            certbotResultModal.hidden = true;
            document.body.classList.remove("modal-open");
            if (complete) window.location.replace("/certbot");
        };
        const load = async (id) => {
            try {
                const response = await fetch(`/certbot/actions/${encodeURIComponent(id)}`, {headers: {"Accept": "application/json"}, credentials: "same-origin", cache: "no-store"});
                if (!response.ok) throw new Error(certText.loadError);
                const result = await response.json();
                const running = result.status === "running";
                const succeeded = result.status === "finished" && result.exit_code === 0;
                complete = !running;
                action.textContent = actionLabels[result.action] || result.action || "—";
                state.textContent = running ? certText.running : succeeded ? certText.success : certText.failed;
                duration.textContent = running ? "—" : `${new Intl.NumberFormat(activeLanguage === "en" ? "en-US" : "fr-FR").format(Number(result.duration_ms) || 0)} ms`;
                exitCode.textContent = result.exit_code === null || result.exit_code === undefined ? "—" : String(result.exit_code);
                status.textContent = running ? certText.stillRunning : succeeded ? certText.done : certText.doneError;
                terminalState.textContent = running ? certText.running : succeeded ? certText.finished : certText.failed;
                const sections = [];
                if (result.stdout) sections.push(`${certText.out}\n${result.stdout}`);
                if (result.stderr) sections.push(`${certText.err}\n${result.stderr}`);
                output.textContent = sections.length ? sections.join("\n\n") : running ? certText.waiting : certText.empty;
                output.scrollTop = output.scrollHeight;
                notice.hidden = !(result.timed_out || result.truncated);
                notice.textContent = result.timed_out ? certText.timeout : result.truncated ? certText.truncated : "";
                if (running) pollingTimer = window.setTimeout(() => load(id), 1000);
            } catch (error) {
                status.textContent = certText.unavailable;
                terminalState.textContent = certText.error;
                notice.hidden = false;
                notice.textContent = error.message || certText.unknown;
            }
        };
        const open = (id) => {
            if (!/^[a-f0-9]{32}$/.test(id || "")) return;
            window.clearTimeout(pollingTimer);
            complete = false;
            status.textContent = certText.loading;
            action.textContent = state.textContent = duration.textContent = exitCode.textContent = "—";
            output.textContent = certText.loadingResult;
            notice.hidden = true;
            certbotResultModal.hidden = false;
            document.body.classList.add("modal-open");
            certbotResultModal.querySelector(".modal__close")?.focus();
            load(id);
        };
        document.addEventListener("click", (event) => {
            const opener = event.target.closest("[data-certbot-result-open]");
            if (opener) open(opener.dataset.certbotResultOpen);
            if (event.target.closest("[data-certbot-result-close]")) close();
        });
        document.addEventListener("keydown", (event) => { if (event.key === "Escape" && !certbotResultModal.hidden) close(); });
        const execution = new URLSearchParams(window.location.search).get("execution");
        if (execution) open(execution);
    }

    document.querySelectorAll("[data-certbot-certificate-action]").forEach((select) => {
        select.addEventListener("change", () => {
            const action = select.value; select.value = ""; if (!action) return;
            const certificate = select.dataset.certificate || "";
            const labels = activeLanguage === "en" ? {reinstall:"Reinstall","renew-replace":"Renew",delete:"Delete"} : {reinstall:"Réinstaller","renew-replace":"Renouveler",delete:"Supprimer"};
            const fallback = activeLanguage === "en" ? "Run the action on" : "Exécuter l’action sur";
            const object = activeLanguage === "en" ? "certificate" : "le certificat";
            if (!window.confirm(`${labels[action] || fallback} ${object} ${certificate} ?`)) return;
            const form = document.createElement("form"); form.method = "post"; form.action = `/certbot/${action}`;
            [["_token", select.dataset.csrf], ["certificate", certificate]].forEach(([name, value]) => { const input = document.createElement("input"); input.type = "hidden"; input.name = name; input.value = value; form.appendChild(input); });
            document.body.appendChild(form); form.submit();
        });
    });

    document.querySelectorAll("[data-firewall-state-form]").forEach((form) => {
        form.addEventListener("submit", (event) => {
            const disabling = form.dataset.firewallState === "disable";
            const message = activeLanguage === "en"
                ? (disabling ? "Disabling the firewall may expose the server. Confirm disabling it?" : "Confirm enabling the firewall? Check that a rule allows your administration access.")
                : (disabling ? "Désactiver le pare-feu peut exposer le serveur. Confirmer la désactivation ?" : "Confirmer l’activation du pare-feu ? Vérifiez qu’une règle autorise votre accès d’administration.");
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
                    if (target) target.textContent = new Intl.NumberFormat(activeLanguage === "en" ? "en-US" : "fr-FR").format(value);
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
    const updateJobParameters = new URLSearchParams(window.location.search);
    const updateJobID = updateJobParameters.get("job");
    if (updateJobModal && /^[a-f0-9]{32}$/.test(updateJobID || "")) {
        const updateText = activeLanguage === "en" ? {success:"Update completed successfully.",failure:"The update failed.",finished:"Completed",failed:"Failed",failureHelp:"Check the last terminal lines to identify the cause of the failure.",noPackage:"No package available.",manager:"Package manager",waiting:"Waiting for the first output…",starting:"Starting the independent service…",running:"Installation in progress… This window will only close when it completes.",preparing:"Preparing",inProgress:"Running",connection:"Update monitoring is temporarily unavailable.",reconnecting:"Reconnecting…",retry:"The job is continuing on the server. Retrying in two seconds."} : {success:"Mise à jour terminée avec succès.",failure:"La mise à jour s’est terminée avec une erreur.",finished:"Terminée",failed:"Échec",failureHelp:"Consultez les dernières lignes du terminal pour identifier la cause de l’échec.",noPackage:"Aucun paquet disponible.",manager:"Gestionnaire de paquets",waiting:"En attente de la première sortie…",starting:"Démarrage du service indépendant…",running:"Installation en cours… Cette fenêtre se fermera uniquement à la fin.",preparing:"Préparation",inProgress:"En cours",connection:"Connexion au suivi momentanément indisponible.",reconnecting:"Reconnexion…",retry:"Le travail continue sur le serveur. Nouvelle tentative dans deux secondes."};
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
            return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat(activeLanguage === "en" ? "en-US" : "fr-FR", {dateStyle: "short", timeStyle: "medium"}).format(date);
        };
        const close = () => {
            if (!complete) return;
            window.clearTimeout(pollingTimer);
            if (updateJobModal.dataset.updateJobResult === "success") {
                window.location.replace(`/updates?refreshed=${Date.now()}`);
                return;
            }
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
            status.textContent = succeeded ? updateText.success : updateText.failure;
            state.textContent = succeeded ? updateText.finished : updateText.failed;
            updateJobModal.dataset.updateJobResult = succeeded ? "success" : "failed";
            notice.hidden = succeeded;
            if (!succeeded) notice.textContent = updateText.failureHelp;
            if (succeeded) {
                document.querySelectorAll("[data-updates-summary] div").forEach((item)=>{const label=item.querySelector("dt")?.textContent.trim();if(label==="Paquets"||label==="Sécurité")item.querySelector("dd").textContent="0";});
                const packageBody=document.querySelector(".updates-stack .table-panel .data-table tbody");
                if(packageBody)packageBody.innerHTML=`<tr><td colspan="5" class="muted">${updateText.noPackage}</td></tr>`;
            }
        };
        const poll = async () => {
            try {
                const response = await fetch(`/updates/jobs/${encodeURIComponent(updateJobID)}`, {headers: {"Accept": "application/json"}, credentials: "same-origin", cache: "no-store"});
                if (!response.ok) throw new Error("status");
                const job = await response.json();
                const lines = Array.isArray(job.lines) ? job.lines : [];
                backend.textContent = String(job.backend || updateText.manager).toUpperCase();
                output.textContent = lines.length ? lines.join("\n") : updateText.waiting;
                output.scrollTop = output.scrollHeight;
                started.textContent = formatDate(job.started_at);
                finished.textContent = formatDate(job.finished_at);
                exitCode.textContent = job.exit_code === null || job.exit_code === undefined ? "—" : String(job.exit_code);
                if (job.status === "pending" || job.status === "running") {
                    const pending = job.status === "pending";
                    status.textContent = pending ? updateText.starting : updateText.running;
                    state.textContent = pending ? updateText.preparing : updateText.inProgress;
                    updateJobModal.dataset.updateJobResult = "running";
                    notice.hidden = true;
                    pollingTimer = window.setTimeout(poll, 1000);
                    return;
                }
                finish(job);
            } catch (_) {
                status.textContent = updateText.connection;
                state.textContent = updateText.reconnecting;
                notice.textContent = updateText.retry;
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

    const rebootWaitModal = document.querySelector("[data-reboot-wait]");
    if (rebootWaitModal?.dataset.rebootActive === "true") {
        const rebootText = activeLanguage === "en" ? {
            stopping: "The reboot is starting. Waiting for the server to stop…",
            offline: "The server is restarting. Reconnection attempts are continuing…",
            online: "The server is available again. Reloading AegisAdmin…"
        } : {
            stopping: "Le redémarrage commence. Attente de l’arrêt du serveur…",
            offline: "Le serveur redémarre. Les tentatives de reconnexion continuent…",
            online: "Le serveur est de nouveau disponible. Rechargement d’AegisAdmin…"
        };
        const status = rebootWaitModal.querySelector("[data-reboot-wait-status]");
        let interruptionObserved = false;
        const probe = async () => {
            const controller = new AbortController();
            const timeout = window.setTimeout(() => controller.abort(), 3000);
            try {
                const response = await fetch(`/healthz?reboot=${Date.now()}`, {cache: "no-store", credentials: "same-origin", signal: controller.signal});
                if (!response.ok) throw new Error("health");
                if (interruptionObserved) {
                    status.textContent = rebootText.online;
                    window.setTimeout(() => window.location.replace(`/updates?reconnected=${Date.now()}`), 500);
                    return;
                }
                status.textContent = rebootText.stopping;
            } catch (_) {
                interruptionObserved = true;
                status.textContent = rebootText.offline;
            } finally {
                window.clearTimeout(timeout);
            }
            window.setTimeout(probe, 2000);
        };
        rebootWaitModal.hidden = false;
        document.body.classList.add("modal-open");
        window.setTimeout(probe, 2000);
    }

    document.querySelectorAll("[data-password-generator]").forEach((form) => {
        const passwordText = activeLanguage === "en" ? {generated:"A random 24-character password was generated and confirmed.",show:"Show",hide:"Hide",copied:"Password copied to the clipboard.",copyFailed:"Automatic copy failed: the password is selected."} : {generated:"Mot de passe aléatoire de 24 caractères généré et confirmé.",show:"Afficher",hide:"Masquer",copied:"Mot de passe copié dans le presse-papiers.",copyFailed:"Copie automatique impossible : le mot de passe est sélectionné."};
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
            status.textContent = passwordText.generated;
            password.focus();
        });
        toggle.addEventListener("click", () => {
            const visible = password.type === "text";
            password.type = visible ? "password" : "text";
            confirmation.type = visible ? "password" : "text";
            toggle.textContent = visible ? passwordText.show : passwordText.hide;
        });
        copy.addEventListener("click", async () => {
            try {
                await navigator.clipboard.writeText(password.value);
                status.textContent = passwordText.copied;
            } catch (_) {
                password.type = "text";
                confirmation.type = "text";
                password.select();
                toggle.textContent = passwordText.hide;
                status.textContent = passwordText.copyFailed;
            }
        });
        password.addEventListener("input", updateCopyState);
    });

    const navigationAdmin = document.querySelector("[data-navigation-admin]");
    if (navigationAdmin) {
        const navigationText = activeLanguage === "en" ? {empty:"Drop a module into this category.",saving:"Saving menu organization…",saved:"Menu organization was saved.",error:"Unable to save."} : {empty:"Déposez un module dans cette catégorie.",saving:"Enregistrement de l’organisation…",saved:"L’organisation du menu a été enregistrée.",error:"Enregistrement impossible."};
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
                    row.innerHTML = `<td class="muted" colspan="5">${navigationText.empty}</td>`;
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
            showNavigationStatus(navigationText.saving, true);
            try {
                const response = await fetch(navigationAdmin.dataset.navigationReorderUrl, {method: "POST", credentials: "same-origin", headers: {"Accept": "application/json", "Content-Type": "application/json"}, body: JSON.stringify({_token: navigationAdmin.dataset.navigationCsrf, ...order})});
                const result = await response.json();
                if (!response.ok || result.success !== true) throw new Error(activeLanguage === "en" ? navigationText.error : (result.message || navigationText.error));
                showNavigationStatus(activeLanguage === "en" ? navigationText.saved : result.message, true);
                window.setTimeout(() => window.location.reload(), 500);
            } catch (error) {
                showNavigationStatus(error instanceof Error ? error.message : navigationText.error, false);
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
