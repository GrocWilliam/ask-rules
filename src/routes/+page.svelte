<script lang="ts">
  import SEO from '$lib/SEO.svelte';
  import { resolve } from '$app/paths';
</script>

<SEO
  title="Reglomatic — Assistant IA pour règles de jeux de société"
  description="Reglomatic répond à vos questions de règles de jeux de société en pleine partie, à partir du livret officiel, avec la page citée."
/>

<main class="page">
  <header class="header">
    <h1 class="logo">Reglomatic</h1>
    <p class="tagline">L'arbitre de vos parties de jeux de société</p>
  </header>

  <section class="about-section">
    <h2><span aria-hidden="true">🎲</span> À quoi ça sert ?</h2>
    <p>
      En pleine partie, une question de règle bloque la table : « Peut-on faire ça ? », « Que fait
      cette tuile ? ». Chercher dans le livret prend du temps et casse le rythme.
    </p>
    <p>
      Reglomatic répond en quelques secondes, en français, à partir du livret de règles officiel du
      jeu. Chaque réponse cite la page du livret : vous pouvez l'ouvrir d'un clic pour vérifier.
    </p>
  </section>

  <section class="about-section">
    <h2><span aria-hidden="true">🧩</span> Quand l'utiliser ?</h2>
    <ul>
      <li><strong>Trancher un désaccord</strong> entre joueurs sur une règle précise.</li>
      <li><strong>Comprendre un élément de jeu</strong> : une carte, une tuile, une action.</li>
      <li>
        <strong>Retrouver un détail oublié</strong> : mise en place, fin de partie, décompte des points.
      </li>
      <li><strong>Apprendre un jeu</strong> : les phases d'un tour, les actions possibles.</li>
    </ul>
  </section>

  <section class="about-section">
    <h2><span aria-hidden="true">♟️</span> Comment l'utiliser ?</h2>
    <ol>
      <li>
        <strong>Choisissez le jeu</strong> dans la liste, ou nommez-le dans votre question : il est reconnu
        automatiquement.
      </li>
      <li>
        <strong>Posez votre question</strong> comme à un autre joueur, au clavier ou à la voix avec le
        micro.
      </li>
      <li>
        <strong>Lisez la réponse</strong> et touchez la page citée pour ouvrir le livret au bon endroit.
        « Voir les passages des règles » montre les extraits utilisés.
      </li>
      <li>
        <strong>Continuez la conversation</strong> : une question de suivi courte (« et pour les bâtiments
        ? ») porte sur le même sujet. « Nouvelle conversation » repart de zéro.
      </li>
    </ol>
  </section>

  <section class="about-section">
    <h2><span aria-hidden="true">💡</span> Bon à savoir</h2>
    <ul>
      <li>
        Les réponses viennent <strong>uniquement du livret</strong> du jeu. Si la règle n'y est pas, Reglomatic
        le dit plutôt que d'inventer.
      </li>
      <li>
        Une IA peut se tromper : en cas de doute, <strong>vérifiez la page citée</strong>.
      </li>
      <li>
        La première question peut prendre plus de temps : le serveur se réveille s'il était en
        veille.
      </li>
      <li>
        L'écran reste allumé pendant la conversation, et l'application s'installe sur votre
        téléphone comme une app.
      </li>
    </ul>
  </section>

  <a href={resolve('/question')} class="submit-btn about-cta">Poser une question</a>
</main>

<style>
  /* Chaque carte prend la couleur d'un pion */
  .about-section {
    background: var(--surface);
    border: 1px solid var(--border);
    border-top: 6px solid var(--pawn);
    border-radius: var(--radius);
    box-shadow: var(--shadow);
    padding: 1.5rem;
  }

  .about-section:nth-of-type(1) {
    --pawn: var(--accent);
  }
  .about-section:nth-of-type(2) {
    --pawn: var(--yellow);
  }
  .about-section:nth-of-type(3) {
    --pawn: var(--green);
  }
  .about-section:nth-of-type(4) {
    --pawn: var(--blue);
  }

  .about-section li::marker {
    color: var(--pawn);
    font-weight: 800;
  }

  .about-section h2 {
    font-size: 1.15rem;
    margin-bottom: 0.75rem;
  }

  .about-section p + p {
    margin-top: 0.75rem;
  }

  .about-section ul,
  .about-section ol {
    padding-left: 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .about-section p,
  .about-section li {
    line-height: 1.6;
  }

  .about-cta {
    align-self: center;
    text-decoration: none;
    font-size: 1.1rem;
    padding: 0.85rem 2rem;
    border-radius: 999px;
    box-shadow: 0 4px 0 var(--accent-hover);
  }
</style>
