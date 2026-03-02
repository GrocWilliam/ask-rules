<script lang="ts">
  import { page } from '$app/stores';

  export let title = 'Reglomatic - Assistant IA pour règles de jeux de société';
  export let description =
    'Posez des questions sur vos jeux de société préférés et obtenez des réponses instantanées grâce à notre IA spécialisée dans les règles de jeux.';
  export let keywords =
    'jeux de société, règles de jeu, IA, assistant intelligent, board games, NLP, questions réponses';
  export let image = '/og-image.svg';
  export let type = 'website';

  $: url = $page.url.href;
  $: siteName = 'Reglomatic';
</script>

<svelte:head>
  <!-- Primary Meta Tags -->
  <title>{title}</title>
  <meta name="title" content={title} />
  <meta name="description" content={description} />
  <meta name="keywords" content={keywords} />
  <meta name="author" content="Reglomatic" />
  <meta name="robots" content="index, follow" />
  <meta name="language" content="French" />
  <link rel="canonical" href={url} />

  <!-- Open Graph / Facebook -->
  <meta property="og:type" content={type} />
  <meta property="og:url" content={url} />
  <meta property="og:title" content={title} />
  <meta property="og:description" content={description} />
  <meta property="og:image" content={image} />
  <meta property="og:site_name" content={siteName} />
  <meta property="og:locale" content="fr_FR" />

  <!-- Twitter -->
  <meta property="twitter:card" content="summary_large_image" />
  <meta property="twitter:url" content={url} />
  <meta property="twitter:title" content={title} />
  <meta property="twitter:description" content={description} />
  <meta property="twitter:image" content={image} />

  <!-- Schema.org for Google -->
  {@html `<script type="application/ld+json">
    ${JSON.stringify({
      '@context': 'https://schema.org',
      '@type': 'WebApplication',
      name: siteName,
      description: description,
      url: url,
      applicationCategory: 'GameApplication',
      operatingSystem: 'Web',
      offers: {
        '@type': 'Offer',
        price: '0',
        priceCurrency: 'EUR',
      },
      inLanguage: 'fr-FR',
    })}
  <\/script>`}
</svelte:head>
