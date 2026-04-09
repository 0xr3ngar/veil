package quotes

import (
	"math/rand"
)

type Quote struct {
	Text   string `json:"text"`
	Source string `json:"source"`
}

var All = []Quote{
	// Scripture
	{"Watch and pray so that you will not fall into temptation. The spirit is willing, but the flesh is weak.", "Matthew 26:41"},
	{"No temptation has overtaken you except what is common to mankind. And God is faithful; he will not let you be tempted beyond what you can bear.", "1 Corinthians 10:13"},
	{"Submit yourselves, then, to God. Resist the devil, and he will flee from you.", "James 4:7"},
	{"I can do all things through Christ who strengthens me.", "Philippians 4:13"},
	{"Be sober-minded; be watchful. Your adversary the devil prowls around like a roaring lion, seeking someone to devour.", "1 Peter 5:8"},
	{"Blessed is the man who remains steadfast under trial, for when he has stood the test he will receive the crown of life.", "James 1:12"},
	{"Create in me a clean heart, O God, and renew a right spirit within me.", "Psalm 51:10"},
	{"The Lord is my shepherd; I shall not want.", "Psalm 23:1"},
	{"Set your minds on things that are above, not on things that are on earth.", "Colossians 3:2"},
	{"For where your treasure is, there your heart will be also.", "Matthew 6:21"},
	{"Be still, and know that I am God.", "Psalm 46:10"},
	{"He who began a good work in you will carry it on to completion until the day of Christ Jesus.", "Philippians 1:6"},
	{"Put on the full armor of God, so that you can take your stand against the devil's schemes.", "Ephesians 6:11"},
	{"The eyes of the Lord are on the righteous, and his ears are attentive to their cry.", "Psalm 34:15"},
	{"Turn my eyes away from worthless things; preserve my life according to your word.", "Psalm 119:37"},
	{"Above all else, guard your heart, for everything you do flows from it.", "Proverbs 4:23"},
	{"Do not conform to the pattern of this world, but be transformed by the renewing of your mind.", "Romans 12:2"},
	{"The Lord is near to all who call on him, to all who call on him in truth.", "Psalm 145:18"},
	{"For God gave us a spirit not of fear but of power and love and self-control.", "2 Timothy 1:7"},
	{"If we confess our sins, he is faithful and just and will forgive us our sins and purify us from all unrighteousness.", "1 John 1:9"},

	// Orthodox Desert Fathers
	{"The soul that loves God finds rest in nothing but God.", "St. Augustine"},
	{"Prayer is the place of refuge for every worry, a foundation for cheerfulness, a source of constant happiness, a protection against sadness.", "St. John Chrysostom"},
	{"Be patient with everyone, but above all with yourself.", "St. Francis de Sales"},
	{"If you are humble, nothing will touch you, neither praise nor disgrace, because you know what you are.", "Mother Teresa"},
	{"Stand at the door of your heart and let no thought enter without questioning it.", "St. Theophan the Recluse"},
	{"Every moment of resistance to temptation is a victory.", "St. John Vianney"},
	{"Acquire the spirit of peace and a thousand souls around you will be saved.", "St. Seraphim of Sarov"},
	{"Do not resent, do not react, keep inner stillness.", "St. Seraphim of Sarov"},
	{"The present moment is the only moment available to us, and it is the door to all moments.", "St. Theophan the Recluse"},
	{"He who has conquered himself is a true warrior.", "The Philokalia"},
	{"Flee temptation and do not stop to reason with it.", "St. Josemaria Escriva"},
	{"You cannot be half a saint; you must be a whole saint or no saint at all.", "St. Therese of Lisieux"},
	{"A clean heart is a free heart. A free heart can love Christ with an undivided love.", "Mother Teresa"},
	{"Prayer is the inner bath of love into which the soul plunges itself.", "St. John Vianney"},
	{"If the heart wanders or is distracted, bring it back to the point quite gently and replace it tenderly in its Master's presence.", "St. Francis de Sales"},
	{"Begin again. Every single day, begin again.", "St. Benedict"},
	{"The way to Christ leads through the desert of temptation.", "The Desert Fathers"},
	{"Struggle, but do not despair. The path is long, but the Lord walks it with you.", "St. Paisios of Mount Athos"},
	{"God does not expect you to be perfect. He expects you to keep trying.", "St. Paisios of Mount Athos"},
	{"When you feel the assault of passions, imagine yourself before the crucified Christ.", "St. Padre Pio"},

	// Orthodox Saints
	{"The one who struggles and endures brings joy to the angels.", "St. Paisios of Mount Athos"},
	{"Do not lose heart. There is hope. Christ is risen.", "Paschal Greeting"},
	{"Let nothing disturb you, let nothing frighten you. All things pass. God does not change.", "St. Teresa of Avila"},
	{"The only true failure is to stop fighting.", "St. Josemaria Escriva"},
	{"Lord Jesus Christ, Son of God, have mercy on me, a sinner.", "The Jesus Prayer"},
	{"Your body is a temple of the Holy Spirit. Honor God with your body.", "1 Corinthians 6:19-20"},
	{"Fall seven times, stand up eight.", "Proverbs (paraphrase)"},
	{"Each day is a new beginning. Treat it that way. Stay away from what might have been, and look at what can be.", "St. Marcia of Rome"},
	{"In the midst of the storm, the Lord says: Peace, be still.", "Mark 4:39"},
	{"The greatest weapon against temptation is prayer.", "The Desert Fathers"},
}

func Random() Quote {
	return All[rand.Intn(len(All))]
}
