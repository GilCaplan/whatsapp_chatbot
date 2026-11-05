package translate

// pack is one downloadable en→language model. Each is pinned here rather than
// looked up in the Argos Translate index, so everyone gets the same model and
// a file that changes is refused. To change one: download it, take its
// `shasum -a 256` and size, and update the line.
type pack struct {
	url    string
	sha256 string
	size   int64
}

// packs covers every language in lang.All except English.
var packs = map[string]pack{
	"he": {"https://argos-net.com/v1/translate-en_he-1_5.argosmodel", "f4b75e20bd51ccef54f2e09d684f40b7f70e3682a6f17ae42dc9d496bbce4d17", 125594693},
	"ar": {"https://argos-net.com/v1/translate-en_ar-1_0.argosmodel", "e2a6e84337f7ebbb55f9dc61dcba3a861c0d786ae5a46fa198fd8d6473f5c775", 88502631},
	"ru": {"https://argos-net.com/v1/translate-en_ru-1_9.argosmodel", "591d743ae103752b88ffc38785c50421320f4eff93c8967e0d3d2e14d4e27811", 195746693},
	"uk": {"https://argos-net.com/v1/translate-en_uk-1_4.argosmodel", "037ebc690c0f7287bff01b09006f76707d7089134205a91ef3d823a3c73811e3", 119889246},
	"es": {"https://argos-net.com/v1/translate-en_es-1_0.argosmodel", "d698d0ef87ad70d5d184b7fa6965905bf4368f09a2bb9ffb165a79bac96af0c4", 87503191},
	"fr": {"https://argos-net.com/v1/translate-en_fr-1_9.argosmodel", "3a65ed83364f4e7b06e30f9dd823db1934899ed3ce839e63f46dc7b09dc797b4", 65472327},
	"de": {"https://argos-net.com/v1/translate-en_de-1_3.argosmodel", "6cd847f0c06c9c66013e6b0932e07fd54a6d90894659c02bf6c5247b72fb25b1", 150508297},
	"it": {"https://argos-net.com/v1/translate-en_it-1_0.argosmodel", "dde2180001a47904ecbbd688a41e35db8a040e4fd5b52e4f29b4bb499516ab32", 87660780},
	"pt": {"https://argos-net.com/v1/translate-en_pt-1_9.argosmodel", "0c5350a2fa5b923de1346edc0d42e08e38bab2e33cede3a0b9a48eb4281ad8a9", 66179184},
	"nl": {"https://argos-net.com/v1/translate-en_nl-1_8.argosmodel", "91406f040619fb519e16af9c304709e9f3195297e626eec6924d62c7c981a33b", 71504559},
	"tr": {"https://argos-net.com/v1/translate-en_tr-1_5.argosmodel", "2d553a00880a0c21dea5b1c375535659e6bf18c9bf2ce9847c2a2fd2ff50316a", 124742526},
	"el": {"https://argos-net.com/v1/translate-en_el-1_9.argosmodel", "ac5a359363b14490a2c6614e6c5ae4a58a8bbbc739b17a4cb3616fe46f2c329b", 69660890},
	"hi": {"https://argos-net.com/v1/translate-en_hi-1_1.argosmodel", "60470a003a9c7339db8c060ed8eafc7cf999ba90dfa5dceebd8d0a4f75f1d3f0", 106752178},
	"th": {"https://argos-net.com/v1/translate-en_th-1_9.argosmodel", "b0fd94eceb04b967b91c2c7d8d1f7b5493c25362e43027a52649242385d0b2a3", 66978916},
	"zh": {"https://argos-net.com/v1/translate-en_zh-1_9.argosmodel", "433e7c4f034d87fbe2353161e05f18646d7999452f801a4e1f0378522b9850ab", 70743021},
	"ja": {"https://argos-net.com/v1/translate-en_ja-1_1.argosmodel", "16300cc4eaa85320520cabcf433b63d01be40ef6966251de72043a083408f716", 120470284},
	"ko": {"https://argos-net.com/v1/translate-en_ko-1_1.argosmodel", "e03d8e65e6d44525ec5808c3409fcf8728c76c2c76925372b6d3dc3278de17fc", 120789009},
}
