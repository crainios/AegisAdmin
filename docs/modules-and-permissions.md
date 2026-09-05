# Modules et droits utilisateurs

Root dispose toujours de tous les droits. Il choisit dans **Utilisateurs** les
droits accordés à chaque autre compte, uniquement parmi les modules actifs.
Le module **À propos** n’est pas attribuable et reste une information générale.

## Niveaux de droits

| Niveau | Effet |
|---|---|
| Aucun | Le module n’apparaît pas dans le menu et ses routes sont refusées. |
| Consultation | Affichage des informations, sans commande modifiant le serveur. |
| Actions | Consultation et commandes d’exploitation réversibles prévues par le module. |
| Modification | Actions, consultation et changements durables de configuration. |

Les niveaux sont cumulatifs : **Modification** inclut **Actions** et
**Consultation** ; **Actions** inclut **Consultation**. Les contrôles sont
réalisés côté serveur : masquer un bouton ne constitue jamais l’unique mesure
de sécurité.

## Modules disponibles

Le tableau de bord et le menu regroupent notamment la supervision système,
Processus, Stockage, Services, Réseau, Journaux, Apache, Certificats TLS,
PHP-FPM, MySQL/MariaDB, Tor, Fail2ban, Pare-feu, Cron, Mises à jour,
Configuration du serveur, Utilisateurs, Modules et Paramètres. Root peut
activer, désactiver, classer et déplacer les modules dans **Modules**.

Certaines commandes exigent un niveau particulier indépendamment de leur
présentation. Par exemple, les opérations de configuration du pare-feu et les
mises à jour nécessitent **Modification**, tandis que les redémarrages de
service relèvent généralement d’**Actions**. Les opérations sensibles réservées
à root, telles que le redémarrage du serveur et l’administration des comptes,
ne sont pas déléguées par un simple droit de module.
