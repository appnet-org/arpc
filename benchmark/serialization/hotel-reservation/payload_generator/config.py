"""Configuration classes for payload generation."""

from dataclasses import dataclass, field
from pathlib import Path
from typing import Dict, Tuple


def _get_default_output_dir() -> str:
    """Get default output directory at the same level as payload_generator/."""
    config_dir = Path(__file__).parent
    return str(config_dir.parent / "payloads")


@dataclass
class DistConfig:
    """Controls array lengths and skew for hotel payloads."""
    hotel_ids_min: int = 1
    hotel_ids_max: int = 20

    hotels_per_result_min: int = 1
    hotels_per_result_max: int = 15

    images_per_hotel_min: int = 1
    images_per_hotel_max: int = 10

    rate_plans_min: int = 1
    rate_plans_max: int = 8

    recommendation_ids_min: int = 1
    recommendation_ids_max: int = 12


@dataclass
class AddressConfig:
    """Hotel address generation (streetNumber, streetName, city, state, country, postalCode, lat, lon)."""
    countries: Tuple[str, ...] = ("US",)
    city_state_pairs: Tuple[Tuple[str, str], ...] = (
        # North Carolina
        ("Durham", "NC"), ("Raleigh", "NC"), ("Charlotte", "NC"), ("Greensboro", "NC"),
        ("Winston-Salem", "NC"), ("Fayetteville", "NC"), ("Cary", "NC"), ("Wilmington", "NC"),
        # Texas
        ("Austin", "TX"), ("Houston", "TX"), ("Dallas", "TX"), ("San Antonio", "TX"),
        ("Fort Worth", "TX"), ("El Paso", "TX"), ("Arlington", "TX"), ("Corpus Christi", "TX"),
        ("Plano", "TX"), ("Laredo", "TX"), ("Lubbock", "TX"), ("Garland", "TX"),
        # Washington
        ("Seattle", "WA"), ("Spokane", "WA"), ("Tacoma", "WA"), ("Vancouver", "WA"),
        ("Bellevue", "WA"), ("Everett", "WA"), ("Kent", "WA"), ("Yakima", "WA"),
        # California
        ("San Jose", "CA"), ("Los Angeles", "CA"), ("San Francisco", "CA"), ("San Diego", "CA"),
        ("Sacramento", "CA"), ("Fresno", "CA"), ("Oakland", "CA"), ("Long Beach", "CA"),
        ("Bakersfield", "CA"), ("Anaheim", "CA"), ("Santa Ana", "CA"), ("Riverside", "CA"),
        ("Stockton", "CA"), ("Irvine", "CA"), ("Chula Vista", "CA"), ("Fremont", "CA"),
        ("San Bernardino", "CA"), ("Modesto", "CA"), ("Oxnard", "CA"), ("Fontana", "CA"),
        # New York
        ("New York", "NY"), ("Buffalo", "NY"), ("Rochester", "NY"), ("Yonkers", "NY"),
        ("Syracuse", "NY"), ("Albany", "NY"), ("New Rochelle", "NY"), ("Mount Vernon", "NY"),
        ("Schenectady", "NY"), ("Utica", "NY"), ("White Plains", "NY"), ("Hempstead", "NY"),
        # Illinois
        ("Chicago", "IL"), ("Aurora", "IL"), ("Naperville", "IL"), ("Joliet", "IL"),
        ("Rockford", "IL"), ("Elgin", "IL"), ("Peoria", "IL"), ("Champaign", "IL"),
        ("Waukegan", "IL"), ("Cicero", "IL"), ("Bloomington", "IL"), ("Arlington Heights", "IL"),
        # Massachusetts
        ("Boston", "MA"), ("Worcester", "MA"), ("Springfield", "MA"), ("Lowell", "MA"),
        ("Cambridge", "MA"), ("New Bedford", "MA"), ("Brockton", "MA"), ("Quincy", "MA"),
        ("Lynn", "MA"), ("Fall River", "MA"), ("Newton", "MA"), ("Lawrence", "MA"),
        # Florida
        ("Miami", "FL"), ("Tampa", "FL"), ("Orlando", "FL"), ("Jacksonville", "FL"),
        ("Tallahassee", "FL"), ("St. Petersburg", "FL"), ("Hialeah", "FL"), ("Port St. Lucie", "FL"),
        ("Cape Coral", "FL"), ("Fort Lauderdale", "FL"), ("Pembroke Pines", "FL"), ("Hollywood", "FL"),
        ("Miramar", "FL"), ("Gainesville", "FL"), ("Coral Springs", "FL"), ("Miami Gardens", "FL"),
        # Pennsylvania
        ("Philadelphia", "PA"), ("Pittsburgh", "PA"), ("Allentown", "PA"), ("Erie", "PA"),
        ("Reading", "PA"), ("Scranton", "PA"), ("Bethlehem", "PA"), ("Lancaster", "PA"),
        ("Harrisburg", "PA"), ("Altoona", "PA"), ("York", "PA"), ("State College", "PA"),
        # Ohio
        ("Columbus", "OH"), ("Cleveland", "OH"), ("Cincinnati", "OH"), ("Toledo", "OH"),
        ("Akron", "OH"), ("Dayton", "OH"), ("Parma", "OH"), ("Canton", "OH"),
        ("Youngstown", "OH"), ("Lorain", "OH"), ("Hamilton", "OH"), ("Springfield", "OH"),
        # Georgia
        ("Atlanta", "GA"), ("Augusta", "GA"), ("Columbus", "GA"), ("Savannah", "GA"),
        ("Athens", "GA"), ("Sandy Springs", "GA"), ("Roswell", "GA"), ("Macon", "GA"),
        ("Johns Creek", "GA"), ("Albany", "GA"), ("Warner Robins", "GA"), ("Alpharetta", "GA"),
        # Michigan
        ("Detroit", "MI"), ("Grand Rapids", "MI"), ("Warren", "MI"), ("Sterling Heights", "MI"),
        ("Lansing", "MI"), ("Ann Arbor", "MI"), ("Flint", "MI"), ("Dearborn", "MI"),
        ("Livonia", "MI"), ("Troy", "MI"), ("Westland", "MI"), ("Farmington Hills", "MI"),
        # North Carolina (additional)
        ("High Point", "NC"), ("Concord", "NC"), ("Asheville", "NC"), ("Gastonia", "NC"),
        # Arizona
        ("Phoenix", "AZ"), ("Tucson", "AZ"), ("Mesa", "AZ"), ("Chandler", "AZ"),
        ("Scottsdale", "AZ"), ("Glendale", "AZ"), ("Gilbert", "AZ"), ("Tempe", "AZ"),
        ("Peoria", "AZ"), ("Surprise", "AZ"), ("Yuma", "AZ"), ("Flagstaff", "AZ"),
        # Tennessee
        ("Nashville", "TN"), ("Memphis", "TN"), ("Knoxville", "TN"), ("Chattanooga", "TN"),
        ("Clarksville", "TN"), ("Murfreesboro", "TN"), ("Franklin", "TN"), ("Jackson", "TN"),
        ("Johnson City", "TN"), ("Bartlett", "TN"), ("Hendersonville", "TN"), ("Kingsport", "TN"),
        # Indiana
        ("Indianapolis", "IN"), ("Fort Wayne", "IN"), ("Evansville", "IN"), ("South Bend", "IN"),
        ("Carmel", "IN"), ("Fishers", "IN"), ("Bloomington", "IN"), ("Hammond", "IN"),
        ("Gary", "IN"), ("Muncie", "IN"), ("Terre Haute", "IN"), ("Kokomo", "IN"),
        # Missouri
        ("Kansas City", "MO"), ("St. Louis", "MO"), ("Springfield", "MO"), ("Columbia", "MO"),
        ("Independence", "MO"), ("Lee's Summit", "MO"), ("O'Fallon", "MO"), ("St. Joseph", "MO"),
        ("St. Charles", "MO"), ("St. Peters", "MO"), ("Blue Springs", "MO"), ("Florissant", "MO"),
        # Maryland
        ("Baltimore", "MD"), ("Frederick", "MD"), ("Rockville", "MD"), ("Gaithersburg", "MD"),
        ("Bowie", "MD"), ("Annapolis", "MD"), ("College Park", "MD"), ("Salisbury", "MD"),
        ("Laurel", "MD"), ("Greenbelt", "MD"), ("Cumberland", "MD"), ("Hagerstown", "MD"),
        # Wisconsin
        ("Milwaukee", "WI"), ("Madison", "WI"), ("Green Bay", "WI"), ("Kenosha", "WI"),
        ("Racine", "WI"), ("Appleton", "WI"), ("Waukesha", "WI"), ("Oshkosh", "WI"),
        ("Eau Claire", "WI"), ("Janesville", "WI"), ("West Allis", "WI"), ("La Crosse", "WI"),
        # Colorado
        ("Denver", "CO"), ("Colorado Springs", "CO"), ("Aurora", "CO"), ("Fort Collins", "CO"),
        ("Lakewood", "CO"), ("Thornton", "CO"), ("Arvada", "CO"), ("Westminster", "CO"),
        ("Pueblo", "CO"), ("Greeley", "CO"), ("Boulder", "CO"), ("Longmont", "CO"),
        # Minnesota
        ("Minneapolis", "MN"), ("St. Paul", "MN"), ("Rochester", "MN"), ("Duluth", "MN"),
        ("Bloomington", "MN"), ("Brooklyn Park", "MN"), ("Plymouth", "MN"), ("St. Cloud", "MN"),
        ("Eagan", "MN"), ("Woodbury", "MN"), ("Maple Grove", "MN"), ("Eden Prairie", "MN"),
        # South Carolina
        ("Charleston", "SC"), ("Columbia", "SC"), ("North Charleston", "SC"), ("Mount Pleasant", "SC"),
        ("Rock Hill", "SC"), ("Greenville", "SC"), ("Summerville", "SC"), ("Sumter", "SC"),
        ("Hilton Head Island", "SC"), ("Spartanburg", "SC"), ("Florence", "SC"), ("Aiken", "SC"),
        # Alabama
        ("Birmingham", "AL"), ("Montgomery", "AL"), ("Mobile", "AL"), ("Huntsville", "AL"),
        ("Tuscaloosa", "AL"), ("Hoover", "AL"), ("Dothan", "AL"), ("Auburn", "AL"),
        ("Decatur", "AL"), ("Madison", "AL"), ("Florence", "AL"), ("Gadsden", "AL"),
        # Louisiana
        ("New Orleans", "LA"), ("Baton Rouge", "LA"), ("Shreveport", "LA"), ("Lafayette", "LA"),
        ("Lake Charles", "LA"), ("Kenner", "LA"), ("Bossier City", "LA"), ("Monroe", "LA"),
        ("Alexandria", "LA"), ("Houma", "LA"), ("Marrero", "LA"), ("Central", "LA"),
        # Kentucky
        ("Louisville", "KY"), ("Lexington", "KY"), ("Bowling Green", "KY"), ("Owensboro", "KY"),
        ("Covington", "KY"), ("Hopkinsville", "KY"), ("Richmond", "KY"), ("Florence", "KY"),
        ("Georgetown", "KY"), ("Henderson", "KY"), ("Elizabethtown", "KY"), ("Jeffersontown", "KY"),
        # Oregon
        ("Portland", "OR"), ("Eugene", "OR"), ("Salem", "OR"), ("Gresham", "OR"),
        ("Hillsboro", "OR"), ("Bend", "OR"), ("Beaverton", "OR"), ("Medford", "OR"),
        ("Springfield", "OR"), ("Corvallis", "OR"), ("Albany", "OR"), ("Tigard", "OR"),
        # Oklahoma
        ("Oklahoma City", "OK"), ("Tulsa", "OK"), ("Norman", "OK"), ("Broken Arrow", "OK"),
        ("Lawton", "OK"), ("Edmond", "OK"), ("Moore", "OK"), ("Midwest City", "OK"),
        ("Enid", "OK"), ("Stillwater", "OK"), ("Muskogee", "OK"), ("Bartlesville", "OK"),
        # Connecticut
        ("Bridgeport", "CT"), ("New Haven", "CT"), ("Hartford", "CT"), ("Stamford", "CT"),
        ("Waterbury", "CT"), ("Norwalk", "CT"), ("Danbury", "CT"), ("New Britain", "CT"),
        ("West Hartford", "CT"), ("Greenwich", "CT"), ("Hamden", "CT"), ("Meriden", "CT"),
        # Utah
        ("Salt Lake City", "UT"), ("West Valley City", "UT"), ("Provo", "UT"), ("West Jordan", "UT"),
        ("Orem", "UT"), ("Sandy", "UT"), ("Ogden", "UT"), ("St. George", "UT"),
        ("Layton", "UT"), ("Taylorsville", "UT"), ("South Jordan", "UT"), ("Lehi", "UT"),
        # Iowa
        ("Des Moines", "IA"), ("Cedar Rapids", "IA"), ("Davenport", "IA"), ("Sioux City", "IA"),
        ("Iowa City", "IA"), ("Waterloo", "IA"), ("Council Bluffs", "IA"), ("Ames", "IA"),
        ("West Des Moines", "IA"), ("Dubuque", "IA"), ("Ankeny", "IA"), ("Urbandale", "IA"),
        # Nevada
        ("Las Vegas", "NV"), ("Henderson", "NV"), ("Reno", "NV"), ("North Las Vegas", "NV"),
        ("Sparks", "NV"), ("Carson City", "NV"), ("Fernley", "NV"), ("Elko", "NV"),
        ("Mesquite", "NV"), ("Boulder City", "NV"), ("Fallon", "NV"), ("Winnemucca", "NV"),
        # Arkansas
        ("Little Rock", "AR"), ("Fort Smith", "AR"), ("Fayetteville", "AR"), ("Jonesboro", "AR"),
        ("North Little Rock", "AR"), ("Conway", "AR"), ("Rogers", "AR"), ("Pine Bluff", "AR"),
        ("Bentonville", "AR"), ("Hot Springs", "AR"), ("Texarkana", "AR"), ("Benton", "AR"),
        # Mississippi
        ("Jackson", "MS"), ("Gulfport", "MS"), ("Southaven", "MS"), ("Hattiesburg", "MS"),
        ("Biloxi", "MS"), ("Meridian", "MS"), ("Tupelo", "MS"), ("Greenville", "MS"),
        ("Olive Branch", "MS"), ("Horn Lake", "MS"), ("Madison", "MS"), ("Ridgeland", "MS"),
        # Kansas
        ("Wichita", "KS"), ("Overland Park", "KS"), ("Kansas City", "KS"), ("Olathe", "KS"),
        ("Topeka", "KS"), ("Lawrence", "KS"), ("Shawnee", "KS"), ("Manhattan", "KS"),
        ("Lenexa", "KS"), ("Salina", "KS"), ("Hutchinson", "KS"), ("Leavenworth", "KS"),
        # New Mexico
        ("Albuquerque", "NM"), ("Las Cruces", "NM"), ("Rio Rancho", "NM"), ("Santa Fe", "NM"),
        ("Roswell", "NM"), ("Farmington", "NM"), ("Clovis", "NM"), ("Hobbs", "NM"),
        ("Alamogordo", "NM"), ("Carlsbad", "NM"), ("Gallup", "NM"), ("Deming", "NM"),
        # Nebraska
        ("Omaha", "NE"), ("Lincoln", "NE"), ("Bellevue", "NE"), ("Grand Island", "NE"),
        ("Kearney", "NE"), ("Fremont", "NE"), ("Hastings", "NE"), ("North Platte", "NE"),
        ("Norfolk", "NE"), ("Columbus", "NE"), ("Papillion", "NE"), ("La Vista", "NE"),
        # West Virginia
        ("Charleston", "WV"), ("Huntington", "WV"), ("Parkersburg", "WV"), ("Morgantown", "WV"),
        ("Wheeling", "WV"), ("Martinsburg", "WV"), ("Fairmont", "WV"), ("Beckley", "WV"),
        ("Clarksburg", "WV"), ("South Charleston", "WV"), ("St. Albans", "WV"), ("Vienna", "WV"),
        # Idaho
        ("Boise", "ID"), ("Nampa", "ID"), ("Meridian", "ID"), ("Idaho Falls", "ID"),
        ("Pocatello", "ID"), ("Caldwell", "ID"), ("Coeur d'Alene", "ID"), ("Twin Falls", "ID"),
        ("Lewiston", "ID"), ("Post Falls", "ID"), ("Rexburg", "ID"), ("Chubbuck", "ID"),
        # Hawaii
        ("Honolulu", "HI"), ("Hilo", "HI"), ("Kailua", "HI"), ("Kaneohe", "HI"),
        ("Pearl City", "HI"), ("Waipahu", "HI"), ("Kahului", "HI"), ("Ewa Beach", "HI"),
        ("Mililani", "HI"), ("Kihei", "HI"), ("Makakilo", "HI"), ("Kailua-Kona", "HI"),
        # New Hampshire
        ("Manchester", "NH"), ("Nashua", "NH"), ("Concord", "NH"), ("Derry", "NH"),
        ("Rochester", "NH"), ("Salem", "NH"), ("Dover", "NH"), ("Goffstown", "NH"),
        ("Londonderry", "NH"), ("Hudson", "NH"), ("Keene", "NH"), ("Portsmouth", "NH"),
        # Maine
        ("Portland", "ME"), ("Lewiston", "ME"), ("Bangor", "ME"), ("South Portland", "ME"),
        ("Auburn", "ME"), ("Biddeford", "ME"), ("Sanford", "ME"), ("Saco", "ME"),
        ("Augusta", "ME"), ("Westbrook", "ME"), ("Waterville", "ME"), ("Presque Isle", "ME"),
        # Rhode Island
        ("Providence", "RI"), ("Warwick", "RI"), ("Cranston", "RI"), ("Pawtucket", "RI"),
        ("East Providence", "RI"), ("Woonsocket", "RI"), ("Newport", "RI"), ("Central Falls", "RI"),
        ("Westerly", "RI"), ("Cumberland", "RI"), ("North Providence", "RI"), ("Johnston", "RI"),
        # Montana
        ("Billings", "MT"), ("Missoula", "MT"), ("Great Falls", "MT"), ("Bozeman", "MT"),
        ("Butte", "MT"), ("Helena", "MT"), ("Kalispell", "MT"), ("Havre", "MT"),
        ("Anaconda", "MT"), ("Miles City", "MT"), ("Livingston", "MT"), ("Laurel", "MT"),
        # Delaware
        ("Wilmington", "DE"), ("Dover", "DE"), ("Newark", "DE"), ("Middletown", "DE"),
        ("Smyrna", "DE"), ("Milford", "DE"), ("Seaford", "DE"), ("Georgetown", "DE"),
        ("Elsmere", "DE"), ("New Castle", "DE"), ("Laurel", "DE"), ("Harrington", "DE"),
        # South Dakota
        ("Sioux Falls", "SD"), ("Rapid City", "SD"), ("Aberdeen", "SD"), ("Brookings", "SD"),
        ("Watertown", "SD"), ("Mitchell", "SD"), ("Yankton", "SD"), ("Pierre", "SD"),
        ("Huron", "SD"), ("Vermillion", "SD"), ("Spearfish", "SD"), ("Madison", "SD"),
        # North Dakota
        ("Fargo", "ND"), ("Bismarck", "ND"), ("Grand Forks", "ND"), ("Minot", "ND"),
        ("West Fargo", "ND"), ("Williston", "ND"), ("Dickinson", "ND"), ("Mandan", "ND"),
        ("Jamestown", "ND"), ("Wahpeton", "ND"), ("Devils Lake", "ND"), ("Valley City", "ND"),
        # Alaska
        ("Anchorage", "AK"), ("Fairbanks", "AK"), ("Juneau", "AK"), ("Sitka", "AK"),
        ("Ketchikan", "AK"), ("Wasilla", "AK"), ("Kenai", "AK"), ("Kodiak", "AK"),
        ("Bethel", "AK"), ("Palmer", "AK"), ("Homer", "AK"), ("Barrow", "AK"),
        # Vermont
        ("Burlington", "VT"), ("Essex", "VT"), ("South Burlington", "VT"), ("Colchester", "VT"),
        ("Rutland", "VT"), ("Montpelier", "VT"), ("Barre", "VT"), ("St. Albans", "VT"),
        ("Brattleboro", "VT"), ("Milton", "VT"), ("Hartford", "VT"), ("Williston", "VT"),
        # Wyoming
        ("Cheyenne", "WY"), ("Casper", "WY"), ("Laramie", "WY"), ("Gillette", "WY"),
        ("Rock Springs", "WY"), ("Sheridan", "WY"), ("Green River", "WY"), ("Evanston", "WY"),
        ("Riverton", "WY"), ("Jackson", "WY"), ("Cody", "WY"), ("Rawlins", "WY"),
    )
    street_names: Tuple[str, ...] = (
        "Main", "Oak", "Park", "Maple", "Cedar", "First", "Second", "Market",
        "Broadway", "Washington", "Lake Shore", "Fifth", "Sunset", "Grand",
        "Pine", "Elm", "Hill", "River", "Lake", "Spring", "Church", "College",
        "State", "Union", "Central", "North", "South", "East", "West",
        "Highland", "Valley", "Ridge", "Forest", "Meadow", "Harbor", "Bay",
        "Jefferson", "Lincoln", "Franklin", "Madison", "Monroe", "Adams",
        "Commerce", "Industrial", "Airport", "Station", "Plaza", "Square",
        "Mill", "Bridge", "View", "Terrace", "Drive", "Boulevard", "Lane",
        "Court", "Circle", "Way", "Place", "Path", "Trail", "Crossing",
    )
    street_suffixes: Tuple[str, ...] = ("St", "Ave", "Blvd", "Dr", "Rd", "Ln", "Ct", "Way", "Pl", "Pkwy")
    zip_min: int = 10000
    zip_max: int = 99999


@dataclass
class HotelTextConfig:
    """Hotel names, descriptions, recommendation requirements, and customer names."""
    name_prefixes: Tuple[str, ...] = (
        "Grand", "Royal", "Plaza", "Continental", "Paramount", "Summit",
        "Riverside", "Lakeside", "Parkview", "City Center", "Downtown",
        "Harbor", "Oceanview", "Mountain", "Valley", "Metro", "Executive",
        "Historic", "Belvedere", "Crown", "Imperial", "Majestic", "Noble",
        "Pacific", "Atlantic", "Gulf", "Sunset", "Sunrise", "Starlight",
        "Garden", "Terrace", "Courtyard", "Villa", "Manor", "Estate",
        "Comfort", "Quality", "Premier", "Select", "Signature", "Boutique",
        "Fairfield", "Hampton", "Holiday", "Hyatt", "Marriott", "Hilton",
        "Westin", "Sheraton", "Radisson", "DoubleTree", "Embassy", "Residence",
        "SpringHill", "TownePlace", "Homewood", "Home2", "Aloft", "W",
        "Ritz", "Waldorf", "Peninsula", "Four Seasons", "Mandarin", "Raffles",
    )
    name_suffixes: Tuple[str, ...] = (
        "Hotel", "Inn", "Suites", "Resort", "Lodge", "House", "Court",
        "Tower", "Place", "Square", "Gardens", "Heights", "Landing",
        "Hotel & Suites", "Hotel & Spa", "Inn & Suites", "Resort & Spa",
        "Conference Center", "Convention Hotel", "Airport Hotel", "Downtown",
    )
    description_templates: Tuple[str, ...] = (
        "A comfortable stay in the heart of the city.",
        "Modern amenities and convenient location.",
        "Upscale accommodation with excellent service.",
        "Family-friendly hotel with pool and breakfast.",
        "Business-friendly with meeting rooms and free WiFi.",
        "Steps from shopping, dining, and entertainment.",
        "Relax in style with premium bedding and city views.",
        "Full-service hotel with 24-hour front desk and fitness center.",
        "Charming property featuring a restaurant and bar.",
        "Ideal for both business and leisure travelers.",
        "Spacious rooms with work desk and complimentary coffee.",
        "Pet-friendly hotel with walking area and treats.",
        "Historic building with modern comforts and character.",
        "Beachfront property with pool and water sports.",
        "Ski-in/ski-out access and hot tubs.",
        "All-suite property with kitchenettes and free breakfast.",
        "Luxury resort with spa, golf, and multiple dining options.",
        "Budget-friendly rooms with free parking and WiFi.",
        "Extended-stay suites with full kitchens.",
        "Boutique hotel with unique design and local art.",
        "Conference facilities and catering for groups.",
        "Rooftop bar and skyline views.",
        "Sustainable hotel with green practices and EV charging.",
        "Waterfront location with marina and boat dock.",
        "Casino hotel with entertainment and dining.",
        "Airport shuttle and early check-in available.",
    )
    recommendation_requirements: Tuple[str, ...] = (
        "nearby", "cheap", "luxury", "family", "business", "quiet",
        "downtown", "airport", "beach", "pool", "pet-friendly", "spa",
        "romantic", "budget", "all-inclusive", "boutique", "historic",
        "waterfront", "mountain", "golf", "wifi", "breakfast", "parking",
    )
    first_names: Tuple[str, ...] = (
        "James", "Mary", "John", "Patricia", "Robert", "Jennifer", "Michael", "Linda",
        "William", "Elizabeth", "David", "Barbara", "Richard", "Susan", "Joseph", "Jessica",
        "Thomas", "Sarah", "Charles", "Karen", "Christopher", "Lisa", "Daniel", "Nancy",
        "Matthew", "Betty", "Anthony", "Margaret", "Mark", "Sandra", "Donald", "Ashley",
        "Steven", "Kimberly", "Paul", "Emily", "Andrew", "Donna", "Joshua", "Michelle",
        "Kenneth", "Dorothy", "Kevin", "Carol", "Brian", "Amanda", "George", "Melissa",
        "Timothy", "Deborah", "Ronald", "Stephanie", "Edward", "Rebecca", "Jason", "Sharon",
        "Jeffrey", "Laura", "Ryan", "Cynthia", "Jacob", "Kathleen", "Gary", "Amy",
        "Nicholas", "Angela", "Eric", "Shirley", "Jonathan", "Anna", "Stephen", "Brenda",
        "Larry", "Pamela", "Justin", "Emma", "Scott", "Nicole", "Brandon", "Helen",
        "Benjamin", "Samantha", "Samuel", "Katherine", "Raymond", "Christine", "Gregory", "Debra",
        "Frank", "Rachel", "Alexander", "Catherine", "Patrick", "Carolyn", "Jack", "Janet",
        "Dennis", "Ruth", "Jerry", "Maria", "Tyler", "Heather", "Aaron", "Diane",
        "Jose", "Virginia", "Adam", "Julie", "Nathan", "Joyce", "Henry", "Victoria",
        "Douglas", "Olivia", "Zachary", "Kelly", "Peter", "Lauren", "Kyle", "Christina",
        "Noah", "Joan", "Ethan", "Evelyn", "Jeremy", "Judith", "Walter", "Megan",
        "Christian", "Andrea", "Keith", "Cheryl", "Roger", "Hannah", "Terry", "Jacqueline",
        "Austin", "Martha", "Sean", "Gloria", "Gerald", "Teresa", "Carl", "Ann",
    )
    last_names: Tuple[str, ...] = (
        "Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia", "Miller", "Davis",
        "Rodriguez", "Martinez", "Hernandez", "Lopez", "Gonzalez", "Wilson", "Anderson", "Thomas",
        "Taylor", "Moore", "Jackson", "Martin", "Lee", "Perez", "Thompson", "White",
        "Harris", "Sanchez", "Clark", "Ramirez", "Lewis", "Robinson", "Walker", "Young",
        "Allen", "King", "Wright", "Scott", "Torres", "Nguyen", "Hill", "Flores",
        "Green", "Adams", "Nelson", "Baker", "Hall", "Rivera", "Campbell", "Mitchell",
        "Carter", "Roberts", "Turner", "Phillips", "Evans", "Parker", "Edwards", "Collins",
        "Stewart", "Morris", "Murphy", "Cook", "Rogers", "Morgan", "Peterson", "Cooper",
        "Reed", "Bailey", "Bell", "Gomez", "Kelly", "Howard", "Ward", "Cox",
        "Diaz", "Richardson", "Wood", "Watson", "Brooks", "Bennett", "Gray", "James",
        "Reyes", "Cruz", "Hughes", "Price", "Myers", "Long", "Foster", "Sanders",
        "Ross", "Morales", "Powell", "Sullivan", "Russell", "Ortiz", "Jenkins", "Gutierrez",
        "Perry", "Butler", "Barnes", "Fisher", "Henderson", "Coleman", "Simmons", "Patterson",
        "Jordan", "Reynolds", "Hamilton", "Graham", "Kim", "Gonzales", "Alexander", "Ramos",
        "Wallace", "Griffin", "West", "Cole", "Hayes", "Chavez", "Gibson", "Bryant",
        "Ellis", "Stevens", "Murray", "Ford", "Marshall", "Owens", "McDonald", "Harrison",
        "Ruiz", "Kennedy", "Wells", "Alvarez", "Woods", "Mendoza", "Castillo", "Olson",
    )
    image_base_url: str = "https://example.com/hotel-images"
    locales: Tuple[str, ...] = (
        "en", "en-US", "en-GB", "fr", "fr-FR", "fr-CA", "es", "es-ES", "es-MX",
        "de", "de-DE", "ja", "ja-JP", "zh", "zh-CN", "zh-TW", "ko", "ko-KR",
        "it", "it-IT", "pt", "pt-BR", "pt-PT", "ru", "ru-RU", "ar", "ar-SA",
        "hi", "hi-IN", "th", "th-TH", "vi", "vi-VN", "id", "id-ID", "nl", "nl-NL",
        "pl", "pl-PL", "tr", "tr-TR", "sv", "sv-SE", "da", "da-DK", "no", "nb-NO",
    )


@dataclass
class RateConfig:
    """Room rates, currencies, and room type descriptions."""
    currencies: Tuple[str, ...] = (
        "USD", "EUR", "JPY", "GBP", "CAD", "AUD", "CHF", "CNY", "HKD", "NZD",
        "SEK", "NOK", "DKK", "SGD", "KRW", "MXN", "INR", "BRL", "ZAR", "RUB",
        "TRY", "PLN", "THB", "IDR", "MYR", "PHP", "CZK", "HUF", "ILS", "CLP",
        "ARS", "AED", "SAR", "QAR", "KWD", "BHD", "OMR", "JOD", "EGP", "NGN",
    )
    bookable_rate_min: float = 80.0
    bookable_rate_max: float = 400.0
    total_rate_min: float = 100.0
    total_rate_max: float = 500.0
    room_codes: Tuple[str, ...] = (
        "STD", "DLX", "STE", "KNG", "QN", "JUN", "EXEC", "PENT",
        "STD2", "DLX2", "OCV", "GV", "SV", "CV", "WF", "BT",
        "STUDIO", "1BR", "2BR", "CONN", "HANDI", "PET", "SMK", "NS",
    )
    room_description_templates: Tuple[str, ...] = (
        "Standard room with queen bed and work desk.",
        "Deluxe room with king bed and city view.",
        "Suite with separate living area and mini bar.",
        "King room with balcony and mountain view.",
        "Queen room with two double beds, ideal for families.",
        "Junior suite with sofa bed and kitchenette.",
        "Executive room with lounge access and breakfast.",
        "Penthouse with panoramic views and butler service.",
        "Standard double with twin beds.",
        "Deluxe double with bay view.",
        "Ocean view room with private terrace.",
        "Garden view with patio access.",
        "Street view room, quiet side.",
        "Corner room with extra windows.",
        "Waterfront room with balcony.",
        "Room with bathtub and rain shower.",
        "Studio with kitchenette and pull-out sofa.",
        "One-bedroom suite with full kitchen.",
        "Two-bedroom suite, sleeps six.",
        "Connecting rooms available.",
        "Accessible room with roll-in shower.",
        "Pet-friendly room with dog bed and bowls.",
        "Smoking room with ashtray.",
        "Non-smoking room with air purifier.",
    )


@dataclass
class OutputConfig:
    out_dir: str = field(default_factory=_get_default_output_dir)
    pretty: bool = False


@dataclass
class Config:
    seed: int = 1
    # Total 100k payloads; distribution reflects typical flow (search/geo/rates heavy, then profile, then reserve/auth).
    counts: Dict[str, int] = field(default_factory=lambda: {
        # Search (most common user action)
        "SearchRequest": 6000,
        "SearchResult": 6500,
        # Geo (nearby)
        "NearbyRequest": 5500,
        "NearbyResult": 5500,
        # Rate (checking prices)
        "GetRatesRequest": 5000,
        "GetRatesResult": 5500,
        "RatePlan": 5500,
        "RoomType": 5500,
        # Profile (viewing hotel details)
        "GetProfilesRequest": 4500,
        "GetProfilesResult": 5000,
        "Hotel": 5500,
        "Address": 5500,
        "Image": 11500,
        # Recommendation
        "GetRecommendationsRequest": 4500,
        "GetRecommendationsResult": 4500,
        # Reservation (fewer complete bookings)
        "ReservationRequest": 3500,
        "ReservationResult": 3500,
        # User (auth)
        "CheckUserRequest": 3500,
        "CheckUserResult": 3500,
    })

    dist: DistConfig = field(default_factory=DistConfig)
    address: AddressConfig = field(default_factory=AddressConfig)
    hotel_text: HotelTextConfig = field(default_factory=HotelTextConfig)
    rate: RateConfig = field(default_factory=RateConfig)
    output: OutputConfig = field(default_factory=OutputConfig)

    hotel_pool_size: int = 2000
    user_pool_size: int = 2000
