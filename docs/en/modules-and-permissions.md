# Modules and user permissions

[Version française](../modules-and-permissions.md)

Root always has every permission. For each other account, root assigns a level
only for active modules. **About** is informational and is not assignable.

| Level | Effect |
|---|---|
| None | The module is hidden and its routes are denied. |
| View | Information is available without server-changing commands. |
| Actions | Includes View and enables the module's operational actions. |
| Modify | Includes Actions and View and enables persistent configuration changes. |

Checks are enforced server-side; hiding a button is never the only security
control. Sensitive operations such as account administration and server reboot
remain root-only where specified.
